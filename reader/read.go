package reader

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/globalmac/idx/writer"
	"io"
	"iter"
	"math"
	"math/big"
	"os"
	"reflect"
	"runtime"
	"strings"
)

const sectionSeparatorSize = 16

const notFound uint = math.MaxUint

type Reader struct {
	nodeReader     nodeReader
	buffer         []byte
	dc             dc
	Metadata       Metadata
	nodeOffsetMult uint
	hasMappedFile  bool
}

type Metadata struct {
	Name       string                  `idx:"name"`
	BuildEpoch uint                    `idx:"created_at"`
	NodeCount  uint                    `idx:"node_count"`
	Total      uint                    `idx:"data_count"`
	Partitions writer.PartitionsConfig `idx:"partitions"`
}

type K3dt byte

type treeNode struct {
	id      uint64
	bit     uint
	pointer uint
}

type Result struct {
	Id     uint64
	err    error
	dc     dc
	offset uint
}

// Open принимает строковый путь к БД и возвращает значение Reader или ошибка.
// Файл БД открывается с использованием карты памяти
// на поддерживаемых платформах. На платформах без поддержки отображения данных в память или если
// попытка сопоставления памяти завершается ошибкой - БД загружается в память.
// Используйте метод Close для объекта Reader, чтобы освободить ресурсы ОС.
func Open(file string) (*Reader, error) {
	mapFile, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer mapFile.Close()

	stats, err := mapFile.Stat()
	if err != nil {
		return nil, err
	}

	size64 := stats.Size()
	// mmapping an empty file returns -EINVAL on Unix platforms,
	// and ERROR_FILE_INVALID on Windows.
	if size64 == 0 {
		return nil, errors.New("file is empty")
	}

	size := int(size64)
	if int64(size) != size64 {
		return nil, errors.New("file too large")
	}

	data, err := mmap(int(mapFile.Fd()), size)
	if err != nil {
		if errors.Is(err, errors.ErrUnsupported) {
			//data, err = openFallback(mapFile, size)
			data = make([]byte, size)
			_, err = io.ReadFull(mapFile, data)
			if err != nil {
				return nil, err
			}
			return OpenRaw(data)
		}
		return nil, err
	}

	reader, err := OpenRaw(data)
	if err != nil {
		_ = munmap(data)
		return nil, err
	}

	reader.hasMappedFile = true
	runtime.SetFinalizer(reader, (*Reader).Close)
	return reader, nil
}

// OpenRaw берет фрагмент байта, соответствующий файлу БД и возвращает структуру считывателя или ошибку.
func OpenRaw(buffer []byte) (*Reader, error) {
	metadataStart := bytes.LastIndex(buffer, writer.HeaderMarker)

	if metadataStart == -1 {
		return nil, newDbError("ошибка при открытии файла БД - некорректный формат файла")
	}

	metadataStart += len(writer.HeaderMarker)
	metadataDecoder := dc{buffer: buffer[metadataStart:]}

	var metadata Metadata

	rvMetadata := reflect.ValueOf(&metadata)
	_, err := metadataDecoder.decode(0, rvMetadata, 0)
	if err != nil {
		return nil, err
	}

	searchTreeSize := metadata.NodeCount * (32 / 4)
	dataSectionStart := searchTreeSize + sectionSeparatorSize
	dataSectionEnd := uint(metadataStart - len(writer.HeaderMarker))
	if dataSectionStart > dataSectionEnd {
		return nil, newDbError("файл БД содержит некорректный формат метаданных")
	}
	d := dc{
		buffer: buffer[searchTreeSize+sectionSeparatorSize : metadataStart-len(writer.HeaderMarker)],
	}

	nb := buffer[:searchTreeSize]
	var nr = nodeReader{buffer: nb}

	reader := &Reader{
		buffer:         buffer,
		nodeReader:     nr,
		dc:             d,
		Metadata:       metadata,
		nodeOffsetMult: 32 / 4,
	}

	return reader, err
}

// Close освобождает ресурсы ОС
func (r *Reader) Close() error {
	var err error
	if r.hasMappedFile {
		runtime.SetFinalizer(r, nil)
		r.hasMappedFile = false
		err = munmap(r.buffer)
	}
	r.buffer = nil
	return err
}

// CheckPartition смотрит вхождение диапазонов в открытом файле и если нет - возвращает партицию, где есть вхождение
func (r *Reader) CheckPartition(id uint64) (bool, bool, string, error) {
	if r.buffer == nil {
		return false, false, "", errors.New("БД недоступна для чтения")
	}

	ranges := r.Metadata.Partitions.Ranges
	n := len(ranges)
	if n == 0 {
		return false, false, "", errors.New("пустой массив партиций")
	}

	// Проверка граничных значений для быстрого выхода
	if id < ranges[0].Min {
		return false, false, "", fmt.Errorf("значение %d находится ниже минимального диапазона", id)
	}
	if id > ranges[n-1].Max {
		return false, false, "", fmt.Errorf("значение %d находится выше максимального диапазона", id)
	}

	// Линейный поиск
	for _, rr := range ranges {
		if id >= rr.Min && id <= rr.Max {
			var sp = fmt.Sprint(rr.Part)
			if r.Metadata.Partitions.Current == rr.Part {
				return true, true, sp, nil
			}
			return true, false, sp, nil
		}
	}

	return false, false, "", fmt.Errorf("значение %d не найдено ни в одной из партиций", id)
}

// GetAllPartitionsFiles возвращает список всех файлов партиции с форматированием
func (r *Reader) GetAllPartitionsFiles(pathStart string, pathSeparator string, pathEnd string) ([]string, error) {

	files := []string{""}

	if r.buffer == nil {
		return nil, errors.New("БД недоступна для чтения")
	}

	if r.Metadata.Partitions.Ranges != nil {
		for _, part := range r.Metadata.Partitions.Ranges {
			files = append(files, pathStart+pathSeparator+fmt.Sprint(part.Part)+pathSeparator+pathEnd)
		}
	}
	return files, nil
}

// Find возвращает 1 узел
func (r *Reader) Find(id uint64) Result {
	if r.buffer == nil {
		return Result{err: errors.New("БД недоступна для чтения")}
	}

	// Предварительно вычисленные константы
	nodeCount := r.Metadata.NodeCount
	offsetMult := r.nodeOffsetMult
	nodeReaders := r.nodeReader

	// Обход дерева
	var node uint
	var prefixLen uint8
	for prefixLen = 0; prefixLen < 64 && node < nodeCount; prefixLen++ {
		// Получаем бит на текущей позиции (0-63)
		bit := (id >> (63 - prefixLen)) & 1

		offset := node * offsetMult
		if bit == 0 {
			node = nodeReaders.readLeft(offset)
		} else {
			node = nodeReaders.readRight(offset)
		}
	}

	// Обработка результатов
	if node == nodeCount {
		// Запись не найдена
		return Result{
			Id:     id,
			offset: notFound,
		}
	} else if node > nodeCount {
		// Нашли указатель на данные
		offset, err := r.resolveDataPointer(node)
		return Result{
			dc:     r.dc,
			Id:     id,
			offset: uint(offset),
			err:    err,
		}
	}

	// Некорректная структура базы данных
	return Result{
		err: newDbError("Некорректная структура узла БД"),
	}
}

// GetAll возвращает итератор по всем узлам
func (r *Reader) GetAll() iter.Seq[Result] {
	return r.Scan(0, 0)
}

// Where ищет записи по вложенному пути и значению с заданным режимом сравнения.
func (r *Reader) Where(path []any, mode string, fieldValue interface{}, yield func(Result) bool) {

	if r.buffer == nil || len(path) == 0 {
		return
	}

	// Поиск по всем элементам среза
	if len(path) == 1 {
		if sliceIndex, ok := path[0].(int); ok && sliceIndex == -1 {
			r.searchAllInSlice(fieldValue, mode, yield)
			return
		} else if mapKey, okm := path[0].(string); okm && mapKey == "*" {
			r.searchAllInMap(fieldValue, mode, yield)
			return
		}
	}

	compareFn := makeComparePathFn(r.dc, fieldValue, mode)
	if compareFn == nil {
		return
	}

	var stack [128]struct {
		node   uint
		nodeID uint64
		bit    uint8
	}
	stackSize := 1
	stack[0] = struct {
		node   uint
		nodeID uint64
		bit    uint8
	}{node: 0, nodeID: 0, bit: 0}

	nodeCount := r.Metadata.NodeCount
	offsetMult := r.nodeOffsetMult

	for stackSize > 0 {
		stackSize--
		item := stack[stackSize]
		node, nodeID, bit := item.node, item.nodeID, item.bit

		if node >= nodeCount {
			if node > nodeCount {
				dataOffset, err := r.resolveDataPointer(node)
				if err != nil {
					continue
				}
				offset := uint(dataOffset)
				valOffset, err := resolvePath(r.dc, offset, path)
				if err != nil {
					continue
				}
				if compareFn(valOffset) {
					if !yield(Result{
						dc:     r.dc,
						Id:     nodeID,
						offset: offset,
					}) {
						return
					}
				}
			}
			continue
		}

		if stackSize+2 < len(stack) {
			offset := node * offsetMult
			leftPointer := r.nodeReader.readLeft(offset)
			rightPointer := r.nodeReader.readRight(offset)

			rightID := nodeID | (1 << (63 - bit))
			nextBit := bit + 1

			stack[stackSize] = struct {
				node   uint
				nodeID uint64
				bit    uint8
			}{node: rightPointer, nodeID: rightID, bit: nextBit}
			stackSize++

			stack[stackSize] = struct {
				node   uint
				nodeID uint64
				bit    uint8
			}{node: leftPointer, nodeID: nodeID, bit: nextBit}
			stackSize++
		}
	}
}

// WhereHas ищет записи по точному значению узла (без пути)
func (r *Reader) WhereHas(fieldValue interface{}, yield func(Result) bool) {
	if r.buffer == nil {
		return
	}

	// Создаем функцию сравнения для конкретного типа
	compareFn := func(offset uint) bool {
		// Определяем тип данных по фактическому значению
		switch v := fieldValue.(type) {
		case string:
			var val string
			_, err := r.dc.decode(offset, reflect.ValueOf(&val), 0)
			return err == nil && val == v
		case float64:
			var val float64
			_, err := r.dc.decode(offset, reflect.ValueOf(&val), 0)
			return err == nil && val == v
		case float32:
			var val float32
			_, err := r.dc.decode(offset, reflect.ValueOf(&val), 0)
			return err == nil && val == v
		case int:
			var val int64
			_, err := r.dc.decode(offset, reflect.ValueOf(&val), 0)
			return err == nil && val == int64(v)
		case bool:
			var val bool
			_, err := r.dc.decode(offset, reflect.ValueOf(&val), 0)
			return err == nil && val == v
		case []byte:
			var val []byte
			_, err := r.dc.decode(offset, reflect.ValueOf(&val), 0)
			return err == nil && bytes.Equal(val, v)
		case uint16:
			var val uint16
			_, err := r.dc.decode(offset, reflect.ValueOf(&val), 0)
			return err == nil && val == v
		case uint32:
			var val uint32
			_, err := r.dc.decode(offset, reflect.ValueOf(&val), 0)
			return err == nil && val == v
		case uint64:
			var val uint64
			_, err := r.dc.decode(offset, reflect.ValueOf(&val), 0)
			return err == nil && val == v
		case *big.Int:
			var val big.Int
			_, err := r.dc.decode(offset, reflect.ValueOf(&val), 0)
			return err == nil && val.Cmp(v) == 0
		default:
			return false
		}
	}

	var stack [128]struct {
		node   uint
		nodeID uint64
		bit    uint8
	}
	stackSize := 1
	stack[0] = struct {
		node   uint
		nodeID uint64
		bit    uint8
	}{node: 0, nodeID: 0, bit: 0}

	nodeCount := r.Metadata.NodeCount
	offsetMult := r.nodeOffsetMult

	for stackSize > 0 {
		stackSize--
		item := stack[stackSize]
		node, nodeID, bit := item.node, item.nodeID, item.bit

		if node >= nodeCount {
			if node > nodeCount {
				dataOffset, err := r.resolveDataPointer(node)
				if err != nil {
					continue
				}

				// Проверяем само значение узла (не путь)
				if compareFn(uint(dataOffset)) {
					if !yield(Result{
						dc:     r.dc,
						Id:     nodeID,
						offset: uint(dataOffset),
					}) {
						return
					}
				}
			}
			continue
		}

		if stackSize+2 < len(stack) {
			offset := node * offsetMult
			leftPointer := r.nodeReader.readLeft(offset)
			rightPointer := r.nodeReader.readRight(offset)

			rightID := nodeID | (1 << (63 - bit))
			nextBit := bit + 1

			stack[stackSize] = struct {
				node   uint
				nodeID uint64
				bit    uint8
			}{node: rightPointer, nodeID: rightID, bit: nextBit}
			stackSize++

			stack[stackSize] = struct {
				node   uint
				nodeID uint64
				bit    uint8
			}{node: leftPointer, nodeID: nodeID, bit: nextBit}
			stackSize++
		}
	}
}

// makeCompareFn возвращает функцию сравнения по смещению и значению / режимы mode - "=", "like", "ilike", "<", ">"
func makeComparePathFn(dc dc, expected interface{}, mode string) func(offset uint) bool {
	switch v := expected.(type) {
	case string:
		switch mode {
		case "=":
			return func(offset uint) bool {
				var val string
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val == v
			}
		case "LIKE":
			return func(offset uint) bool {
				var val string
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && strings.Contains(val, v)
			}
		case "ILIKE":
			expectedLower := strings.ToLower(v)
			return func(offset uint) bool {
				var val string
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && strings.Contains(strings.ToLower(val), expectedLower)
			}
		}

	case float64:
		switch mode {
		case "=":
			return func(offset uint) bool {
				var val float64
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val == v
			}
		case "<":
			return func(offset uint) bool {
				var val float64
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val < v
			}
		case ">":
			return func(offset uint) bool {
				var val float64
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val > v
			}
		}

	case float32:
		switch mode {
		case "=":
			return func(offset uint) bool {
				var val float32
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val == v
			}
		case "<":
			return func(offset uint) bool {
				var val float32
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val < v
			}
		case ">":
			return func(offset uint) bool {
				var val float32
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val > v
			}
		}

	case bool:
		if mode != "=" {
			return nil
		}
		return func(offset uint) bool {
			var val bool
			_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
			return err == nil && val == v
		}

	case int:
		vInt := int64(v)
		switch mode {
		case "=":
			return func(offset uint) bool {
				var val int64
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val == vInt
			}
		case "!=":
			return func(offset uint) bool {
				var val int64
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val != vInt
			}
		case "<":
			return func(offset uint) bool {
				var val int64
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val < vInt
			}
		case ">":
			return func(offset uint) bool {
				var val int64
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val > vInt
			}
		}

	case []byte:
		if mode != "=" {
			return nil
		}
		return func(offset uint) bool {
			var val []byte
			_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
			return err == nil && bytes.Equal(val, v)
		}

	case uint16:
		switch mode {
		case "=":
			return func(offset uint) bool {
				var val uint16
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val == v
			}
		case "!=":
			return func(offset uint) bool {
				var val uint16
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val != v
			}
		case "<":
			return func(offset uint) bool {
				var val uint16
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val < v
			}
		case ">":
			return func(offset uint) bool {
				var val uint16
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val > v
			}
		}

	case uint32:
		switch mode {
		case "=":
			return func(offset uint) bool {
				var val uint32
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val == v
			}
		case "!=":
			return func(offset uint) bool {
				var val uint32
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val != v
			}
		case "<":
			return func(offset uint) bool {
				var val uint32
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val < v
			}
		case ">":
			return func(offset uint) bool {
				var val uint32
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val > v
			}
		}

	case uint64:
		switch mode {
		case "=":
			return func(offset uint) bool {
				var val uint64
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val == v
			}
		case "!=":
			return func(offset uint) bool {
				var val uint64
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val != v
			}
		case "<":
			return func(offset uint) bool {
				var val uint64
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val < v
			}
		case ">":
			return func(offset uint) bool {
				var val uint64
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val > v
			}
		}

	case *big.Int:
		switch mode {
		case "=":
			return func(offset uint) bool {
				var val big.Int
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val.Cmp(v) == 0
			}
		case "!=":
			return func(offset uint) bool {
				var val big.Int
				_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
				return err == nil && val.Cmp(v) != 0
			}
		}

	case []string:
		if mode != "IN" {
			return nil
		}
		set := make(map[string]struct{}, len(v))
		for _, item := range v {
			set[item] = struct{}{}
		}
		return func(offset uint) bool {
			var val string
			_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
			_, ok := set[val]
			return err == nil && ok
		}

	case []int:
		if mode != "IN" {
			return nil
		}
		set := make(map[int64]struct{}, len(v))
		for _, item := range v {
			set[int64(item)] = struct{}{}
		}
		return func(offset uint) bool {
			var val int64
			_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
			_, ok := set[val]
			return err == nil && ok
		}

	case []uint64:
		if mode != "IN" {
			return nil
		}
		set := make(map[uint64]struct{}, len(v))
		for _, item := range v {
			set[item] = struct{}{}
		}
		return func(offset uint) bool {
			var val uint64
			_, err := dc.decode(offset, reflect.ValueOf(&val), 0)
			_, ok := set[val]
			return err == nil && ok
		}
	}
	return nil
}

// resolvePath проходит по вложенному пути (map[string] / []any) и возвращает смещение
func resolvePath(dc dc, offset uint, path []any) (uint, error) {
	curr := offset
	for _, part := range path {
		switch key := part.(type) {
		case string:
			newOffset, err := findInMap(dc, curr, key)
			if err != nil {
				return 0, err
			}
			curr = newOffset
		case int:
			newOffset, err := findInSlice(dc, curr, uint(key))
			if err != nil {
				return 0, err
			}
			curr = newOffset
		default:
			return 0, errors.New("неподдерживаемый путь элемента")
		}
	}
	return curr, nil
}

// findInMap ищет ключ в Map по смещению
func findInMap(dc dc, offset uint, key string) (uint, error) {
	typeNum, size, newOffset, err := dc.decodeCtrlData(offset)
	if err != nil || typeNum != writer.TypeMap {
		return 0, errors.New("не Map")
	}
	curr := newOffset
	for i := uint(0); i < size; i++ {
		k, valOffset, err := dc.decodeKey(curr)
		if err != nil {
			return 0, err
		}
		if string(k) == key {
			return valOffset, nil
		}
		curr, err = dc.nextValueOffset(valOffset, 1)
		if err != nil {
			return 0, err
		}
	}
	return 0, errors.New("ключа нет в Map")
}

// findInSlice ищет индекс в Slice
func findInSlice(dc dc, offset uint, index uint) (uint, error) {
	typeNum, size, curr, err := dc.decodeCtrlData(offset)
	if err != nil || typeNum != writer.TypeSlice {
		return 0, errors.New("не Slice")
	}
	if index >= size {
		return 0, errors.New("индекс вне диапазона")
	}
	for i := uint(0); i < index; i++ {
		curr, err = dc.nextValueOffset(curr, 1)
		if err != nil {
			return 0, err
		}
	}
	return curr, nil
}

// searchAllInSlice ищет заданное значение во всех значениях Slice
func (r *Reader) searchAllInSlice(fieldValue interface{}, mode string, yield func(Result) bool) {
	compareFn := makeComparePathFn(r.dc, fieldValue, mode)
	if compareFn == nil {
		return
	}

	var stack [128]struct {
		node   uint
		nodeID uint64
		bit    uint8
	}
	stackSize := 1
	stack[0] = struct {
		node   uint
		nodeID uint64
		bit    uint8
	}{node: 0, nodeID: 0, bit: 0}

	nodeCount := r.Metadata.NodeCount
	offsetMult := r.nodeOffsetMult

	for stackSize > 0 {
		stackSize--
		item := stack[stackSize]
		node, nodeID, bit := item.node, item.nodeID, item.bit

		if node >= nodeCount {
			if node > nodeCount {
				dataOffset, err := r.resolveDataPointer(node)
				if err != nil {
					continue
				}
				offset := uint(dataOffset)

				// Исправленный вызов decodeCtrlData с передачей всех параметров
				typeNum, size, newOffset, err := r.dc.decodeCtrlData(offset)
				if err != nil || typeNum != writer.TypeSlice {
					continue
				}

				// Проверяем все элементы среза
				curr := newOffset
				for i := uint(0); i < size; i++ {
					if compareFn(curr) {
						if !yield(Result{
							dc:     r.dc,
							Id:     nodeID,
							offset: offset,
						}) {
							return
						}
						break
					}

					// Получаем смещение следующего элемента
					nextOffset, err := r.dc.nextValueOffset(curr, 1)
					if err != nil {
						break
					}
					curr = nextOffset
				}
			}
			continue
		}

		if stackSize+2 < len(stack) {
			offset := node * offsetMult
			leftPointer := r.nodeReader.readLeft(offset)
			rightPointer := r.nodeReader.readRight(offset)

			rightID := nodeID | (1 << (63 - bit))
			nextBit := bit + 1

			stack[stackSize] = struct {
				node   uint
				nodeID uint64
				bit    uint8
			}{node: rightPointer, nodeID: rightID, bit: nextBit}
			stackSize++

			stack[stackSize] = struct {
				node   uint
				nodeID uint64
				bit    uint8
			}{node: leftPointer, nodeID: nodeID, bit: nextBit}
			stackSize++
		}
	}
}

// searchAllInMap ищет заданное значение во всех значениях Map (не только по ключам)
func (r *Reader) searchAllInMap(fieldValue interface{}, mode string, yield func(Result) bool) {
	compareFn := makeComparePathFn(r.dc, fieldValue, mode)
	if compareFn == nil {
		return
	}

	var stack [128]struct {
		node   uint
		nodeID uint64
		bit    uint8
	}
	stackSize := 1
	stack[0] = struct {
		node   uint
		nodeID uint64
		bit    uint8
	}{node: 0, nodeID: 0, bit: 0}

	nodeCount := r.Metadata.NodeCount
	offsetMult := r.nodeOffsetMult

	for stackSize > 0 {
		stackSize--
		item := stack[stackSize]
		node, nodeID, bit := item.node, item.nodeID, item.bit

		if node >= nodeCount {
			if node > nodeCount {
				dataOffset, err := r.resolveDataPointer(node)
				if err != nil {
					continue
				}
				offset := uint(dataOffset)

				typeNum, size, newOffset, err := r.dc.decodeCtrlData(offset)
				if err != nil || typeNum != writer.TypeMap {
					continue
				}

				// Проверяем все значения Map (пропускаем ключи)
				curr := newOffset
				for i := uint(0); i < size; i++ {
					// Пропускаем ключ
					_, valOffset, err := r.dc.decodeKey(curr)
					if err != nil {
						break
					}

					// Проверяем значение
					if compareFn(valOffset) {
						if !yield(Result{
							dc:     r.dc,
							Id:     nodeID,
							offset: offset,
						}) {
							return
						}
						break
					}

					// Переходим к следующей паре ключ-значение
					curr, err = r.dc.nextValueOffset(valOffset, 1)
					if err != nil {
						break
					}
				}
			}
			continue
		}

		if stackSize+2 < len(stack) {
			offset := node * offsetMult
			leftPointer := r.nodeReader.readLeft(offset)
			rightPointer := r.nodeReader.readRight(offset)

			rightID := nodeID | (1 << (63 - bit))
			nextBit := bit + 1

			stack[stackSize] = struct {
				node   uint
				nodeID uint64
				bit    uint8
			}{node: rightPointer, nodeID: rightID, bit: nextBit}
			stackSize++

			stack[stackSize] = struct {
				node   uint
				nodeID uint64
				bit    uint8
			}{node: leftPointer, nodeID: nodeID, bit: nextBit}
			stackSize++
		}
	}
}

///

func (r *Reader) Scan(id uint64, prefixLen uint8) iter.Seq[Result] {
	return func(yield func(Result) bool) {

		stopBit := int(prefixLen)
		if stopBit > 64 {
			stopBit = 64
		}

		pointer, bit := r.traverseTree(id, 0, stopBit)

		mask := uint64(0xFFFFFFFF) << (64 - bit)
		network := id & mask

		nodes := []treeNode{{
			id:      network,
			bit:     uint(bit),
			pointer: pointer,
		}}

		for len(nodes) > 0 {
			node := nodes[len(nodes)-1]
			nodes = nodes[:len(nodes)-1]

			for {
				if node.pointer == r.Metadata.NodeCount {
					break
				}

				if node.pointer > r.Metadata.NodeCount {
					offset, err := r.resolveDataPointer(node.pointer)
					ok := yield(Result{
						dc:     r.dc,
						Id:     node.id,
						offset: uint(offset),
						err:    err,
					})
					if !ok {
						return
					}
					break
				}

				// Создаем правую ветку с установленным битом
				idRight := node.id | (1 << (62 - node.bit))

				offset := node.pointer * r.nodeOffsetMult
				rightPointer := r.nodeReader.readRight(offset)

				node.bit++
				nodes = append(nodes, treeNode{
					pointer: rightPointer,
					id:      idRight,
					bit:     node.bit,
				})

				node.pointer = r.nodeReader.readLeft(offset)
			}
		}
	}
}

func (r *Reader) GetRange(start, end uint64) iter.Seq[Result] {
	return func(yield func(Result) bool) {
		if start > end {
			return
		}

		// Предварительно вычисленные константы
		nodeCount := r.Metadata.NodeCount
		offsetMult := r.nodeOffsetMult
		nr := r.nodeReader

		// Фиксированный стек (достаточно для 32 уровней глубины)
		var stack [64]struct {
			node   uint
			nodeID uint64
			bit    uint8
		}
		stackSize := 1
		stack[0] = struct {
			node   uint
			nodeID uint64
			bit    uint8
		}{node: 0, nodeID: 0, bit: 0}

		for stackSize > 0 {
			// Извлекаем текущий элемент
			stackSize--
			item := stack[stackSize]
			node, nodeID, bit := item.node, item.nodeID, item.bit

			// Вычисляем границы текущей сети
			mask := ^uint64(0) << (64 - bit)
			netStart := nodeID & mask
			netEnd := netStart | ^mask

			// Проверяем пересечение с нашим диапазоном
			if netStart > end || netEnd < start {
				continue
			}

			if node >= nodeCount {
				if node != nodeCount {
					offset, err := r.resolveDataPointer(node)
					if !yield(Result{
						dc:     r.dc,
						Id:     netStart,
						offset: uint(offset),
						//prefixLen: bit,
						err: err,
					}) {
						return
					}
				}
				continue
			}

			// Вычисляем адрес для правой ветки
			rightBit := bit + 1
			rightID := netStart | (1 << (63 - bit))

			// Читаем указатели на потомков
			offset := node * offsetMult
			leftPointer := nr.readLeft(offset)
			rightPointer := nr.readRight(offset)

			// Добавляем в стек (сначала правую ветку, потом левую)
			if rightID <= end {
				if stackSize < len(stack) {
					stack[stackSize] = struct {
						node   uint
						nodeID uint64
						bit    uint8
					}{node: rightPointer, nodeID: rightID, bit: rightBit}
					stackSize++
				}
			}

			if netStart <= end {
				if stackSize < len(stack) {
					stack[stackSize] = struct {
						node   uint
						nodeID uint64
						bit    uint8
					}{node: leftPointer, nodeID: netStart, bit: rightBit}
					stackSize++
				}
			}
		}
	}
}

func (r *Reader) traverseTree(id uint64, node uint, stopBit int) (uint, int) {
	nodeCount := r.Metadata.NodeCount

	for i := 0; i < stopBit && node < nodeCount; i++ {
		// Получаем бит на позиции i (0-63)
		bit := (id >> (63 - i)) & 1

		offset := node * r.nodeOffsetMult
		if bit == 0 {
			node = r.nodeReader.readLeft(offset)
		} else {
			node = r.nodeReader.readRight(offset)
		}
	}

	return node, stopBit
}

func (r *Reader) resolveDataPointer(pointer uint) (uintptr, error) {
	resolved := uintptr(pointer - r.Metadata.NodeCount - sectionSeparatorSize)

	if resolved >= uintptr(len(r.buffer)) {
		return 0, newDbError("нарушена структура поискового дерева в БД")
	}
	return resolved, nil
}

func (r Result) Decode(v any) error {
	if r.err != nil {
		return r.err
	}
	if r.offset == notFound {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("result param must be a pointer")
	}

	if dser, ok := v.(deserializer); ok {
		_, err := r.dc.decodeToDeserializer(r.offset, dser, 0, false)
		return err
	}

	_, err := r.dc.decode(r.offset, rv, 0)
	return err
}

/*func (r Result) ID() uint64 {
	if r.Exist() {
		return r.Id
	}
	return 0
}*/

func (r Result) DecodePath(v any, path ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.offset == notFound {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("result param must be a pointer")
	}
	return r.dc.decodePath(r.offset, path, rv)
}

func (r Result) Err() error {
	return r.err
}

func (r Result) Exist() bool {
	return r.err == nil && r.offset != notFound
}

type deserializer interface {
	ShouldSkip(offset uintptr) (bool, error)
	StartSlice(size uint) error
	StartMap(size uint) error
	End() error
	String(string) error
	Float64(float64) error
	Bytes([]byte) error
	Uint16(uint16) error
	Uint32(uint32) error
	Int32(int32) error
	Uint64(uint64) error
	Uint128(*big.Int) error
	Bool(bool) error
	Float32(float32) error
}

type nodeReader struct {
	buffer []byte
}

func (n nodeReader) readLeft(nodeNumber uint) uint {
	return (uint(n.buffer[nodeNumber]) << 24) |
		(uint(n.buffer[nodeNumber+1]) << 16) |
		(uint(n.buffer[nodeNumber+2]) << 8) |
		uint(n.buffer[nodeNumber+3])
}

func (n nodeReader) readRight(nodeNumber uint) uint {
	return (uint(n.buffer[nodeNumber+4]) << 24) |
		(uint(n.buffer[nodeNumber+5]) << 16) |
		(uint(n.buffer[nodeNumber+6]) << 8) |
		uint(n.buffer[nodeNumber+7])
}
