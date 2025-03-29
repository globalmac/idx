package writer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash"
	"io"
	"math/big"
	"math/bits"
	"reflect"
	"slices"
)

// K3dt представляет тип данных для сериализации
type K3dt byte

// Константы типов данных
const (
	TypeExtended K3dt = iota
	TypePointer
	TypeString
	TypeFloat64
	TypeBytes
	TypeUint16
	TypeUint32
	TypeMap
	TypeInt32
	TypeUint64
	TypeUint128
	TypeSlice
	TypeBool
	TypeFloat32
)

// Константы для размеров данных
const (
	recordEmpty recordType = iota
	recordData
	recordNode

	sizeSmall  = 29
	sizeMedium = sizeSmall + 256
	sizeLarge  = sizeMedium + (1 << 16)
	sizeMax    = sizeLarge + (1 << 24)

	refSize0 = 1 << 11
	refSize1 = refSize0 + (1 << 19)
	refSize2 = refSize1 + (1 << 27)
)

// hashBuffer объединяет буфер и хеш-функцию
type hashBuffer struct {
	*bytes.Buffer
	hashed hash.Hash
	//digest [sha256.Size]byte
	digest [32]byte
}

// MergeFunc представляет функцию для слияния данных
type MergeFunc func(DataItem) (DataItem, error)

// recordType представляет тип записи в дереве
type recordType byte

// record представляет запись в узле дерева
type record struct {
	node  *node      // Узел
	value *dataEntry // Значение
	rType recordType // Тип записи
}

// node представляет узел дерева
type node struct {
	children [2]record // Дочерние узлы
	id       int       // Идентификатор
}

// insertOps представляет операцию вставки
type insertOps struct {
	mergeFunc  func(value DataItem) (DataItem, error) // Функция слияния
	storage    *storage                               // Хранилище данных
	key        uint32                                 // Ключ
	prefixBits int                                    // Количество бит префикса
	rType      recordType                             // Тип записи
}

// Интерфейсы

// serializer объединяет методы для сериализации данных
type serializer interface {
	io.Writer
	WriteByte(byte) error
	WriteString(string) (int, error)
	WriteOrRef(DataItem) (int64, error)
}

// DataItem представляет интерфейс для всех сериализуемых данных
type DataItem interface {
	Copy() DataItem                      // Создает копию элемента
	Equal(DataItem) bool                 // Сравнивает элементы
	Size() int                           // Возвращает размер данных
	Type() K3dt                          // Возвращает тип данных
	Serialize(serializer) (int64, error) // Сериализует данные
}

// Базовые типы данных

// DataBool представляет булево значение
type DataBool bool

func (t DataBool) Copy() DataItem { return t }

func (t DataBool) Equal(other DataItem) bool {
	o, ok := other.(DataBool)
	return ok && t == o
}

func (t DataBool) Size() int {
	if t {
		return 1
	}
	return 0
}

func (t DataBool) Type() K3dt {
	return TypeBool
}

func (t DataBool) Serialize(w serializer) (int64, error) {
	return writeTypeHeader(w, t)
}

// DataBytes представляет байтовый массив
type DataBytes []byte

func (t DataBytes) Copy() DataItem {
	n := make(DataBytes, len(t))
	copy(n, t)
	return n
}

func (t DataBytes) Equal(other DataItem) bool {
	o, ok := other.(DataBytes)
	return ok && bytes.Equal(t, o)
}

func (t DataBytes) Size() int {
	return len(t)
}

func (t DataBytes) Type() K3dt {
	return TypeBytes
}

func (t DataBytes) Serialize(w serializer) (int64, error) {
	n, err := writeTypeHeader(w, t)
	if err != nil {
		return n, err
	}
	written, err := w.Write(t)
	return n + int64(written), err
}

// DataString представляет строку
type DataString string

func (t DataString) Copy() DataItem { return t }

func (t DataString) Equal(other DataItem) bool {
	o, ok := other.(DataString)
	return ok && t == o
}

func (t DataString) Size() int {
	return len(t)
}

func (t DataString) Type() K3dt {
	return TypeString
}

func (t DataString) Serialize(w serializer) (int64, error) {
	n, err := writeTypeHeader(w, t)
	if err != nil {
		return n, err
	}
	written, err := w.WriteString(string(t))
	return n + int64(written), err
}

// Числовые типы

// DataFloat32 представляет 32-битное число с плавающей точкой
type DataFloat32 float32

func (t DataFloat32) Copy() DataItem { return t }

func (t DataFloat32) Equal(other DataItem) bool {
	o, ok := other.(DataFloat32)
	return ok && t == o
}

func (t DataFloat32) Size() int {
	return 4
}

func (t DataFloat32) Type() K3dt {
	return TypeFloat32
}

func (t DataFloat32) Serialize(w serializer) (int64, error) {
	n, err := writeTypeHeader(w, t)
	if err != nil {
		return n, err
	}
	err = binary.Write(w, binary.BigEndian, t)
	return n + 4, err
}

// DataFloat64 представляет 64-битное число с плавающей точкой
type DataFloat64 float64

func (t DataFloat64) Copy() DataItem { return t }

func (t DataFloat64) Equal(other DataItem) bool {
	o, ok := other.(DataFloat64)
	return ok && t == o
}

func (t DataFloat64) Size() int {
	return 8
}

func (t DataFloat64) Type() K3dt {
	return TypeFloat64
}

func (t DataFloat64) Serialize(w serializer) (int64, error) {
	n, err := writeTypeHeader(w, t)
	if err != nil {
		return n, err
	}
	err = binary.Write(w, binary.BigEndian, t)
	return n + 8, err
}

// DataInt32 представляет 32-битное целое число со знаком
type DataInt32 int32

func (t DataInt32) Copy() DataItem { return t }

func (t DataInt32) Equal(other DataItem) bool {
	o, ok := other.(DataInt32)
	return ok && t == o
}

func (t DataInt32) Size() int {
	return 4 - bits.LeadingZeros32(uint32(t))/8
}

func (t DataInt32) Type() K3dt {
	return TypeInt32
}

func (t DataInt32) Serialize(w serializer) (int64, error) {
	n, err := writeTypeHeader(w, t)
	if err != nil {
		return n, err
	}
	size := t.Size()
	for i := size; i > 0; i-- {
		if err = w.WriteByte(byte(t >> (8 * (i - 1)) & 0xFF)); err != nil {
			return n + int64(size-i), err
		}
	}
	return n + int64(size), nil
}

// DataUint16 представляет 16-битное целое число без знака
type DataUint16 uint16

func (t DataUint16) Copy() DataItem { return t }

func (t DataUint16) Equal(other DataItem) bool {
	o, ok := other.(DataUint16)
	return ok && t == o
}

func (t DataUint16) Size() int {
	return 2 - bits.LeadingZeros16(uint16(t))/8
}

func (t DataUint16) Type() K3dt {
	return TypeUint16
}

func (t DataUint16) Serialize(w serializer) (int64, error) {
	n, err := writeTypeHeader(w, t)
	if err != nil {
		return n, err
	}
	size := t.Size()
	for i := size; i > 0; i-- {
		if err = w.WriteByte(byte(t >> (8 * (i - 1)) & 0xFF)); err != nil {
			return n + int64(size-i), err
		}
	}
	return n + int64(size), nil
}

// DataUint32 представляет 32-битное целое число без знака
type DataUint32 uint32

func (t DataUint32) Copy() DataItem { return t }

func (t DataUint32) Equal(other DataItem) bool {
	o, ok := other.(DataUint32)
	return ok && t == o
}

func (t DataUint32) Size() int {
	return 4 - bits.LeadingZeros32(uint32(t))/8
}

func (t DataUint32) Type() K3dt {
	return TypeUint32
}

func (t DataUint32) Serialize(w serializer) (int64, error) {
	n, err := writeTypeHeader(w, t)
	if err != nil {
		return n, err
	}
	size := t.Size()
	for i := size; i > 0; i-- {
		if err = w.WriteByte(byte(t >> (8 * (i - 1)) & 0xFF)); err != nil {
			return n + int64(size-i), err
		}
	}
	return n + int64(size), nil
}

// DataUint64 представляет 64-битное целое число без знака
type DataUint64 uint64

func (t DataUint64) Copy() DataItem { return t }

func (t DataUint64) Equal(other DataItem) bool {
	o, ok := other.(DataUint64)
	return ok && t == o
}

func (t DataUint64) Size() int {
	return 8 - bits.LeadingZeros64(uint64(t))/8
}

func (t DataUint64) Type() K3dt {
	return TypeUint64
}

func (t DataUint64) Serialize(w serializer) (int64, error) {
	n, err := writeTypeHeader(w, t)
	if err != nil {
		return n, err
	}
	size := t.Size()
	for i := size; i > 0; i-- {
		if err = w.WriteByte(byte(t >> (8 * (i - 1)) & 0xFF)); err != nil {
			return n + int64(size-i), err
		}
	}
	return n + int64(size), nil
}

// DataUint128 представляет 128-битное целое число без знака
type DataUint128 big.Int

func (t *DataUint128) Copy() DataItem {
	n := big.Int{}
	n.Set((*big.Int)(t))
	return (*DataUint128)(&n)
}

func (t *DataUint128) Equal(other DataItem) bool {
	o, ok := other.(*DataUint128)
	return ok && (*big.Int)(t).Cmp((*big.Int)(o)) == 0
}

func (t *DataUint128) Size() int {
	return ((*big.Int)(t).BitLen() + 7) / 8
}

func (t *DataUint128) Type() K3dt {
	return TypeUint128
}

func (t *DataUint128) Serialize(w serializer) (int64, error) {
	n, err := writeTypeHeader(w, t)
	if err != nil {
		return n, err
	}
	written, err := w.Write((*big.Int)(t).Bytes())
	return n + int64(written), err
}

// Составные типы данных

// DataMap представляет словарь (ключ-значение)
type DataMap map[DataString]DataItem

func (t DataMap) Copy() DataItem {
	m := make(DataMap, len(t))
	for k, v := range t {
		m[k] = v.Copy()
	}
	return m
}

func (t DataMap) Equal(other DataItem) bool {
	o, ok := other.(DataMap)
	if !ok || len(t) != len(o) {
		return false
	}
	if reflect.ValueOf(t).Pointer() == reflect.ValueOf(o).Pointer() {
		return true
	}
	for k, v := range t {
		if ov, ok := o[k]; !ok || !v.Equal(ov) {
			return false
		}
	}
	return true
}

func (t DataMap) Size() int {
	return len(t)
}

func (t DataMap) Type() K3dt {
	return TypeMap
}

func (t DataMap) Serialize(w serializer) (int64, error) {
	n, err := writeTypeHeader(w, t)
	if err != nil {
		return n, err
	}

	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, string(k))
	}
	slices.Sort(keys)

	for _, k := range keys {
		key := DataString(k)
		written, err := w.WriteOrRef(key)
		n += written
		if err != nil {
			return n, err
		}
		written, err = w.WriteOrRef(t[key])
		n += written
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// DataSlice представляет срез элементов
type DataSlice []DataItem

func (t DataSlice) Copy() DataItem {
	s := make(DataSlice, len(t))
	for i, v := range t {
		s[i] = v.Copy()
	}
	return s
}

func (t DataSlice) Equal(other DataItem) bool {
	o, ok := other.(DataSlice)
	if !ok || len(t) != len(o) {
		return false
	}
	if reflect.ValueOf(t).Pointer() == reflect.ValueOf(o).Pointer() {
		return true
	}
	for i, v := range t {
		if !v.Equal(o[i]) {
			return false
		}
	}
	return true
}

func (t DataSlice) Size() int {
	return len(t)
}

func (t DataSlice) Type() K3dt {
	return TypeSlice
}

func (t DataSlice) Serialize(w serializer) (int64, error) {
	n, err := writeTypeHeader(w, t)
	if err != nil {
		return n, err
	}
	for _, item := range t {
		written, err := w.WriteOrRef(item)
		n += written
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// RefMarker представляет ссылку на ранее сериализованные данные
type RefMarker uint32

func (t RefMarker) Copy() DataItem { return t }

func (t RefMarker) Equal(other DataItem) bool {
	o, ok := other.(RefMarker)
	return ok && t == o
}

func (t RefMarker) Size() int {
	switch {
	case t < refSize0:
		return 0
	case t < refSize1:
		return 1
	case t < refSize2:
		return 2
	default:
		return 3
	}
}

func (t RefMarker) SerializedSize() int64 {
	return int64(t.Size() + 2)
}

func (t RefMarker) Type() K3dt {
	return TypePointer
}

func (t RefMarker) Serialize(w serializer) (int64, error) {
	size := t.Size()
	switch size {
	case 0:
		if err := w.WriteByte(0b00100000 | byte(0b111&(t>>8))); err != nil {
			return 0, err
		}
		if err := w.WriteByte(byte(0xFF & t)); err != nil {
			return 1, err
		}
	case 1:
		v := t - refSize0
		if err := w.WriteByte(0b00101000 | byte(0b111&(v>>16))); err != nil {
			return 0, err
		}
		if err := w.WriteByte(byte(0xFF & (v >> 8))); err != nil {
			return 1, err
		}
		if err := w.WriteByte(byte(0xFF & v)); err != nil {
			return 2, err
		}
	case 2:
		v := t - refSize1
		if err := w.WriteByte(0b00110000 | byte(0b111&(v>>24))); err != nil {
			return 0, err
		}
		if err := w.WriteByte(byte(0xFF & (v >> 16))); err != nil {
			return 1, err
		}
		if err := w.WriteByte(byte(0xFF & (v >> 8))); err != nil {
			return 2, err
		}
		if err := w.WriteByte(byte(0xFF & v)); err != nil {
			return 3, err
		}
	case 3:
		if err := w.WriteByte(0b00111000); err != nil {
			return 0, err
		}
		if err := w.WriteByte(byte(0xFF & (t >> 24))); err != nil {
			return 1, err
		}
		if err := w.WriteByte(byte(0xFF & (t >> 16))); err != nil {
			return 2, err
		}
		if err := w.WriteByte(byte(0xFF & (t >> 8))); err != nil {
			return 3, err
		}
		if err := w.WriteByte(byte(0xFF & t)); err != nil {
			return 4, err
		}
	}
	return t.SerializedSize(), nil
}

// Вспомогательные функции

// writeTypeHeader записывает заголовок типа данных перед самими данными
func writeTypeHeader(w serializer, t DataItem) (int64, error) {
	size := t.Size()
	typeN := t.Type()

	var b1, b2 byte

	if typeN < 8 {
		b1 = byte(typeN << 5)
	} else {
		b1 = byte(TypeExtended << 5)
		b2 = byte(typeN - 7)
	}

	var extra int
	var extraSize int
	switch {
	case size < sizeSmall:
		b1 |= byte(size)
	case size < sizeMedium:
		b1 |= 29
		extra = size - sizeSmall
		extraSize = 1
	case size < sizeLarge:
		b1 |= 30
		extra = size - sizeMedium
		extraSize = 2
	case size < sizeMax:
		b1 |= 31
		extra = size - sizeLarge
		extraSize = 3
	default:
		return 0, fmt.Errorf("размер %d превысил макс. %d", size, sizeMax-1)
	}

	if err := w.WriteByte(b1); err != nil {
		return 0, err
	}
	n := int64(1)

	if b2 != 0 {
		if err := w.WriteByte(b2); err != nil {
			return n, err
		}
		n++
	}

	for i := extraSize - 1; i >= 0; i-- {
		b := byte((extra >> (8 * i)) & 0xFF)
		if err := w.WriteByte(b); err != nil {
			return n, err
		}
		n++
	}

	return n, nil
}
