package writer

import (
	"bytes"
	"fmt"
	"math"
)

// dataSerializer сериализует данные с поддержкой ссылок
type dataSerializer struct {
	*bytes.Buffer
	storage    *storage               // Хранилище данных
	positions  map[dataKey]storedItem // Позиции элементов
	hashBuffer *hashBuffer            // Буфер для хеширования
}

// writeWithRef записывает данные с использованием ссылок
func (ds *dataSerializer) writeWithRef(e *dataEntry) (int, error) {
	stored, exists := ds.positions[e.key]
	if exists {
		return int(stored.ref), nil
	}

	pos := ds.Len()
	length, err := e.value.Serialize(ds)
	if err != nil {
		return 0, err
	}

	if pos > math.MaxUint32 {
		return 0, fmt.Errorf("позиция %d превысила макс. допустимое значение", pos)
	}

	stored = storedItem{
		ref:    RefMarker(pos),
		length: length,
	}
	ds.positions[e.key] = stored

	return int(stored.ref), nil
}

// WriteOrRef записывает данные или использует ссылку, если возможно
func (ds *dataSerializer) WriteOrRef(t DataItem) (int64, error) {
	keyBytes, err := ds.hashBuffer.Hash(t)
	if err != nil {
		return 0, err
	}

	var exists bool
	var stored storedItem

	stored, exists = ds.positions[dataKey(keyBytes)]
	if exists && stored.length > stored.ref.SerializedSize() {
		return stored.ref.Serialize(ds)
	}

	key := dataKey(keyBytes)
	pos := ds.Len()
	length, err := t.Serialize(ds)
	if err != nil || exists {
		return length, err
	}

	if pos > math.MaxUint32 {
		return 0, fmt.Errorf("позиция %d превысила макс. допустимое значение", pos)
	}

	ds.positions[key] = storedItem{
		ref:    RefMarker(pos),
		length: length,
	}
	return length, nil
}

// WriteOrRef сериализует DataItem в буфер
func (hb *hashBuffer) WriteOrRef(t DataItem) (int64, error) {
	return t.Serialize(hb)
}

// newDataSerializer создает новый сериализатор данных
func newDataSerializer(ds *storage) *dataSerializer {
	return &dataSerializer{
		Buffer:     &bytes.Buffer{},
		storage:    ds,
		positions:  make(map[dataKey]storedItem),
		hashBuffer: newHashBuffer(),
	}
}
