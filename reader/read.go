package reader

import (
	"bytes"
	"errors"
	"github.com/globalmac/idx/writer"
	"io"
	"iter"
	"math"
	"math/big"
	"os"
	"reflect"
	"runtime"
)

// var sectionSeparatorSize = 16
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
	Name       string `idx:"name"`
	BuildEpoch uint   `idx:"created_at"`
	NodeCount  uint   `idx:"node_count"`
}

type K3dt byte

type treeNode struct {
	id      uint64
	bit     uint
	pointer uint
}

type Result struct {
	id     uint64
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
			id:     id,
			offset: notFound,
		}
	} else if node > nodeCount {
		// Нашли указатель на данные
		offset, err := r.resolveDataPointer(node)
		return Result{
			dc:     r.dc,
			id:     id,
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

func (r *Reader) Where(fieldName string, fieldValue interface{}, yield func(Result) bool) {
	if r.buffer == nil {
		return
	}

	var expectedKind reflect.Kind
	switch fieldValue.(type) {
	case string:
		expectedKind = reflect.String
	case uint32:
		expectedKind = reflect.Uint32
	case uint64:
		expectedKind = reflect.Uint64
	case int:
		expectedKind = reflect.Int
	case bool:
		expectedKind = reflect.Bool
	case float64:
		expectedKind = reflect.Float64
	default:
		return
	}

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

				// Исправление: преобразуем uintptr в uint для работы с декодером
				offset := uint(dataOffset)
				typeNum, size, newOffset, err := r.dc.decodeCtrlData(offset)
				if err != nil {
					continue
				}

				if typeNum == writer.TypeMap {
					found := false
					currentOffset := newOffset
					for i := uint(0); i < size; i++ {
						key, nextOffset, err := r.dc.decodeKey(currentOffset)
						if err != nil {
							break
						}

						if string(key) == fieldName {
							fieldOffset := nextOffset
							match := false

							switch expectedKind {
							case reflect.String:
								var val string
								_, err := r.dc.decode(fieldOffset, reflect.ValueOf(&val).Elem(), 0)
								match = err == nil && val == fieldValue.(string)
							case reflect.Uint32:
								var val uint32
								_, err := r.dc.decode(fieldOffset, reflect.ValueOf(&val).Elem(), 0)
								match = err == nil && val == fieldValue.(uint32)
							case reflect.Uint64:
								var val uint64
								_, err := r.dc.decode(fieldOffset, reflect.ValueOf(&val).Elem(), 0)
								match = err == nil && val == fieldValue.(uint64)
							}

							if match {
								if !yield(Result{
									dc:     r.dc,
									id:     nodeID,
									offset: offset,
									//prefixLen: bit,
								}) {
									return
								}
							}
							found = true
							break
						}

						// Исправление: работаем с uint для nextValueOffset
						currentOffset, err = r.dc.nextValueOffset(nextOffset, 1)
						if err != nil {
							break
						}
					}
					if found {
						continue
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
						id:     node.id,
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
						id:     netStart,
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
