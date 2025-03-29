package writer

import (
	"bytes"
	"fmt"
)

// KeyHashFunc интерфейс для хеширования ключей
type KeyHashFunc interface {
	Hash(DataItem) ([]byte, error)
}

// storage представляет хранилище данных
type storage struct {
	entries   map[dataKey]*dataEntry // Записи
	keyHasher KeyHashFunc            // Функция хеширования
}

// dataKey представляет ключ для хранения данных
type dataKey string

// dataEntry представляет запись в хранилище данных
type dataEntry struct {
	value    DataItem // Значение
	key      dataKey  // Ключ
	refCount uint32   // Счетчик ссылок
}

// storedItem представляет сохраненный элемент со ссылкой и длиной
type storedItem struct {
	ref    RefMarker // Ссылка
	length int64     // Длина
}

// newHashBuffer создает новый hashBuffer
func newHashBuffer() *hashBuffer {
	return &hashBuffer{
		Buffer: &bytes.Buffer{},
		//hashed: sha256.New(),
		hashed: Murmur3(),
	}
}

// Hash вычисляет хеш для DataItem
func (hb *hashBuffer) Hash(v DataItem) ([]byte, error) {
	hb.Reset()
	hb.hashed.Reset()
	if _, err := v.Serialize(hb); err != nil {
		return nil, err
	}
	if _, err := hb.WriteTo(hb.hashed); err != nil {
		return nil, fmt.Errorf("хешируемые данные: %w", err)
	}
	return hb.hashed.Sum(hb.digest[:0]), nil
	//return hb.hashed.Sum(nil), nil
}

// newStorage создает новое хранилище данных
func newStorage(keyHasher KeyHashFunc) *storage {
	return &storage{
		entries:   make(map[dataKey]*dataEntry),
		keyHasher: keyHasher,
	}
}

// add добавляет DataItem в хранилище
func (ds *storage) add(v DataItem) (*dataEntry, error) {
	key, err := ds.keyHasher.Hash(v)
	if err != nil {
		return nil, err
	}

	entry, exists := ds.entries[dataKey(key)]
	if !exists {
		entry = &dataEntry{
			key:   dataKey(key),
			value: v,
		}
		ds.entries[dataKey(key)] = entry
	}

	entry.refCount++
	return entry, nil
}

// remove уменьшает счетчик ссылок и удаляет запись при необходимости
func (ds *storage) remove(e *dataEntry) {
	if e == nil {
		return
	}
	e.refCount--

	if e.refCount == 0 {
		delete(ds.entries, e.key)
	}
}
