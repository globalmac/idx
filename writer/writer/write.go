package writer

import (
	"bufio"
	"fmt"
	"io"
	"time"
)

// Константы для маркеров и разделителей
var (
	HeaderMarker     = []byte("~IDX")
	SectionSeparator = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
)

// Config содержит настройки для создания бинарного дерева
type Config struct {
	Timestamp int64       // Временная метка
	Name      string      // Название БД
	KeyHasher KeyHashFunc // Функция хеширования ключей
}

// New создает новое бинарное дерево с заданной конфигурацией
func New(cfg Config) (*BinaryTree, error) {
	tree := &BinaryTree{
		timestamp: time.Now().Unix(),
		nodeSize:  32,
		root:      &node{},
		depth:     32,
	}

	if cfg.Timestamp != 0 {
		tree.timestamp = cfg.Timestamp
	}

	if cfg.Name != "" {
		tree.name = cfg.Name
	}

	tree.storage = newStorage(newHashBuffer())
	//tree.nodeSize = 32

	return tree, nil
}

// Insert добавляет новый элемент в дерево по ключу
func (t *BinaryTree) Insert(key uint64, data DataItem) error {
	t.totalNodes = 0
	t.totalSize++
	return t.root.insert(
		insertOps{
			key:        key,
			prefixBits: 64,
			rType:      recordData,
			mergeFunc: func(value DataItem) MergeFunc {
				return func(_ DataItem) (DataItem, error) {
					return value, nil
				}
			}(data),
			storage: t.storage,
		},
		0,
	)
}

// Find ищет элемент в дереве по ключу
func (t *BinaryTree) Find(key uint64) (uint64, DataItem) {
	prefixBits, r := t.root.find(key, 0)

	var mask uint64 = 0
	if prefixBits != 0 {
		mask = ^uint64(0) << (64 - prefixBits)
	}
	matched := key & mask

	var data = r.value.value
	return matched, data
}

// Serialize сериализует дерево в writer
func (t *BinaryTree) Serialize(w io.Writer) (int64, error) {
	if t.totalNodes == 0 {
		t.prepare()
	}

	buf := bufio.NewWriter(w)
	defer buf.Flush()

	nodeBuf := make([]byte, 2*t.nodeSize/8)
	dataWriter := newDataSerializer(t.storage)

	nodeCount, bytesWritten, err := t.writeNode(buf, t.root, dataWriter, nodeBuf)
	if err != nil {
		return bytesWritten, err
	}
	if nodeCount != t.totalNodes {
		return bytesWritten, fmt.Errorf(
			"узлов записано (%d) != ожидаемым (%d)",
			nodeCount,
			t.totalNodes,
		)
	}

	n, err := buf.Write(SectionSeparator)
	bytesWritten += int64(n)
	if err != nil {
		return bytesWritten, fmt.Errorf("записи размедилетя секций: %w", err)
	}

	n64, err := dataWriter.WriteTo(buf)
	bytesWritten += n64
	if err != nil {
		return bytesWritten, fmt.Errorf("записи секции данных: %w", err)
	}

	n, err = buf.Write(HeaderMarker)
	bytesWritten += int64(n)
	if err != nil {
		return bytesWritten, fmt.Errorf("записи маркера заголовка: %w", err)
	}

	metaWriter := newDataSerializer(dataWriter.storage)
	_, err = t.writeMetadata(metaWriter)
	if err != nil {
		return bytesWritten, fmt.Errorf("записи метаданных: %w", err)
	}

	n64, err = metaWriter.WriteTo(buf)
	bytesWritten += n64
	if err != nil {
		return bytesWritten, fmt.Errorf("записи секции метаданных: %w", err)
	}

	err = buf.Flush()
	if err != nil {
		return bytesWritten, fmt.Errorf("сброса буффера: %w", err)
	}

	return bytesWritten, nil
}
