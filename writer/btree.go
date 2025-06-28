package writer

import (
	"fmt"
	"io"
	"math"
)

// BinaryTree представляет бинарное дерево для хранения данных
type BinaryTree struct {
	timestamp  int64    // Временная метка
	name       string   // Название БД
	storage    *storage // Хранилище данных
	nodeSize   int      // Размер узла
	root       *node    // Корневой узел
	totalNodes int      // Общее количество узлов
	depth      int      // Глубина дерева
	totalSize  int      // Общее кол-во элементов в записи
}

// prepare подготавливает дерево к сериализации
func (t *BinaryTree) prepare() {
	t.totalNodes = t.root.prepare(0)
}

// writeNode записывает узел дерева
func (t *BinaryTree) writeNode(w io.Writer, node *node, dw *dataSerializer, nodeBuf []byte) (int, int64, error) {
	if err := t.encodeNode(nodeBuf, node, dw); err != nil {
		return 0, 0, err
	}

	bytesWritten := int64(0)
	rbytes, err := w.Write(nodeBuf)
	bytesWritten += int64(rbytes)
	nodesWritten := 1
	if err != nil {
		return nodesWritten, bytesWritten, fmt.Errorf("записи узла: %w", err)
	}

	for i := range 2 {
		child := node.children[i]
		if child.rType != recordNode {
			continue
		}
		addedNodes, addedBytes, err := t.writeNode(
			w,
			node.children[i].node,
			dw,
			nodeBuf,
		)
		nodesWritten += addedNodes
		bytesWritten += addedBytes
		if err != nil {
			return nodesWritten, bytesWritten, err
		}
	}

	return nodesWritten, bytesWritten, nil
}

// nodeValue возвращает значение узла
func (t *BinaryTree) nodeValue(r record, dw *dataSerializer) (int, error) {
	switch r.rType {
	case recordData:
		pos, err := dw.writeWithRef(r.value)
		return t.totalNodes + len(SectionSeparator) + pos, err
	case recordEmpty:
		return t.totalNodes, nil
	default:
		return r.node.id, nil
	}
}

// encodeNode кодирует узел в байтовый буфер
func (t *BinaryTree) encodeNode(buf []byte, n *node, dataWriter *dataSerializer) error {
	left, err := t.nodeValue(n.children[0], dataWriter)
	if err != nil {
		return err
	}
	right, err := t.nodeValue(n.children[1], dataWriter)
	if err != nil {
		return err
	}

	maxNodeValue := 1 << t.nodeSize
	if left >= maxNodeValue || right >= maxNodeValue {
		return fmt.Errorf(
			"значения узлов (%d, %d) превысили лимит в %d-бит",
			left,
			right,
			t.nodeSize,
		)
	}

	switch t.nodeSize {
	case 24:
		buf[0] = byte((left >> 16) & 0xFF)
		buf[1] = byte((left >> 8) & 0xFF)
		buf[2] = byte(left & 0xFF)
		buf[3] = byte((right >> 16) & 0xFF)
		buf[4] = byte((right >> 8) & 0xFF)
		buf[5] = byte(right & 0xFF)
	case 28:
		buf[0] = byte((left >> 16) & 0xFF)
		buf[1] = byte((left >> 8) & 0xFF)
		buf[2] = byte(left & 0xFF)
		buf[3] = byte((((left >> 24) & 0x0F) << 4) | (right >> 24 & 0x0F))
		buf[4] = byte((right >> 16) & 0xFF)
		buf[5] = byte((right >> 8) & 0xFF)
		buf[6] = byte(right & 0xFF)
	case 32:
		buf[0] = byte((left >> 24) & 0xFF)
		buf[1] = byte((left >> 16) & 0xFF)
		buf[2] = byte((left >> 8) & 0xFF)
		buf[3] = byte(left & 0xFF)
		buf[4] = byte((right >> 24) & 0xFF)
		buf[5] = byte((right >> 16) & 0xFF)
		buf[6] = byte((right >> 8) & 0xFF)
		buf[7] = byte(right & 0xFF)
	default:
		return fmt.Errorf("неподдерживаемый размер узла: %d", t.nodeSize)
	}
	return nil
}

// writeMetadata записывает метаданные дерева
func (t *BinaryTree) writeMetadata(ds *dataSerializer) (int64, error) {
	if t.totalNodes > math.MaxUint32 {
		return 0, fmt.Errorf("кол-во узлов %d превысило максимум", t.totalNodes)
	}
	meta := DataMap{
		"created_at": DataUint64(t.timestamp),
		"name":       DataString(t.name),
		"node_count": DataUint32(t.totalNodes),
		"data_count": DataUint32(t.totalSize),
	}
	return meta.Serialize(ds)
}

// insert вставляет запись в узел дерева
func (n *node) insert(op insertOps, depth int) error {
	newDepth := depth + 1
	if newDepth > op.prefixBits {
		err := n.children[0].insert(op, newDepth)
		if err != nil {
			return err
		}
		return n.children[1].insert(op, newDepth)
	}

	pos := getBit(op.key, depth)
	r := &n.children[pos]
	return r.insert(op, newDepth)
}

// insert вставляет запись в дерево
func (r *record) insert(op insertOps, depth int) error {
	switch r.rType {
	case recordNode:
		err := r.node.insert(op, depth)
		if err != nil {
			return err
		}
		return r.tryMerge(op)
	case recordEmpty, recordData:
		if depth >= op.prefixBits {
			//r.node = op.targetNode
			r.rType = op.rType
			if op.rType == recordData {
				var oldVal DataItem
				if r.value != nil {
					oldVal = r.value.value
				}
				newVal, err := op.mergeFunc(oldVal)
				if err != nil {
					return err
				}
				if newVal == nil {
					op.storage.remove(r.value)
					r.rType = recordEmpty
					r.value = nil
				} else if oldVal == nil || !oldVal.Equal(newVal) {
					op.storage.remove(r.value)
					val, err := op.storage.add(newVal)
					if err != nil {
						return err
					}
					r.value = val
				}
			} else {
				r.value = nil
			}
			return nil
		}

		r.node = &node{children: [2]record{*r, *r}}
		r.value = nil
		r.rType = recordNode
		err := r.node.insert(op, depth)
		if err != nil {
			return err
		}
		return r.tryMerge(op)
	default:
		return fmt.Errorf("неподдерживаемый тип записи: %d", r.rType)
	}
}

// tryMerge пытается объединить дочерние записи
func (r *record) tryMerge(op insertOps) error {
	child0 := r.node.children[0]
	child1 := r.node.children[1]
	if child0.rType != child1.rType {
		return nil
	}
	switch child0.rType {
	case recordNode:
		return nil
	case recordEmpty:
		r.rType = child0.rType
		r.node = nil
		return nil
	case recordData:
		if child0.value.key != child1.value.key {
			return nil
		}
		r.rType = recordData
		r.value = child0.value
		op.storage.remove(child1.value)
		r.node = nil
		return nil
	default:
		return fmt.Errorf("не удалось объединить тип записи: %d", child0.rType)
	}
}

// find ищет запись по ключу
func (n *node) find(key uint64, depth int) (int, record) {
	r := n.children[getBit(key, depth)]
	depth++

	switch r.rType {
	case recordNode:
		return r.node.find(key, depth)
	default:
		return depth, r
	}
}

// prepare подготавливает узлы дерева к сериализации
func (n *node) prepare(currentID int) int {
	n.id = currentID
	currentID++

	for i := range 2 {
		switch n.children[i].rType {
		case recordNode:
			currentID = n.children[i].node.prepare(currentID)
		default:
		}
	}

	return currentID
}

// getBit возвращает бит на указанной позиции
func getBit(value uint64, pos int) byte {
	return byte((value >> (63 - pos)) & 1)
}
