package reader

import (
	"fmt"
	"reflect"
	"runtime"
)

type validator struct {
	reader *Reader
}

func (r *Reader) Validate() error {
	v := validator{r}
	if err := v.validateMetadata(); err != nil {
		return err
	}

	err := v.validateDatabase()
	runtime.KeepAlive(v.reader)
	return err
}

func (v *validator) validateMetadata() error {
	metadata := v.reader.Metadata

	if metadata.NodeCount == 0 {
		return fmt.Errorf(
			"%v - Expected: %v Actual: %v",
			"node_count",
			"positive integer",
			metadata.NodeCount,
		)
	}
	return nil
}

func (v *validator) validateDatabase() error {
	offsets, err := v.validateTree()
	if err != nil {
		return err
	}

	if err := v.validateDataSectionSeparator(); err != nil {
		return err
	}

	return v.validateDataSection(offsets)
}

func (v *validator) validateTree() (map[uint]bool, error) {
	offsets := make(map[uint]bool)

	for result := range v.reader.GetAll() {
		if err := result.Err(); err != nil {
			return nil, err
		}
		offsets[result.offset] = true
	}
	return offsets, nil
}

func (v *validator) validateDataSectionSeparator() error {
	separatorStart := v.reader.Metadata.NodeCount * 32 / 4

	separator := v.reader.buffer[separatorStart : separatorStart+sectionSeparatorSize]

	for _, b := range separator {
		if b != 0 {
			return newDbError("unexpected byte in data separator: %v", separator)
		}
	}
	return nil
}

func (v *validator) validateDataSection(offsets map[uint]bool) error {
	pointerCount := len(offsets)

	dcr := v.reader.dc

	var offset uint
	bufferLen := uint(len(dcr.buffer))
	for offset < bufferLen {
		var data any
		rv := reflect.ValueOf(&data)
		newOffset, err := dcr.decode(offset, rv, 0)
		if err != nil {
			return newDbError(
				"received decoding error (%v) at offset of %v",
				err,
				offset,
			)
		}
		if newOffset <= offset {
			return newDbError(
				"data section offset unexpectedly went from %v to %v",
				offset,
				newOffset,
			)
		}

		pointer := offset

		if _, ok := offsets[pointer]; !ok {
			return newDbError(
				"found data (%v) at %v that the search tree does not point to",
				data,
				pointer,
			)
		}
		delete(offsets, pointer)

		offset = newOffset
	}

	if offset != bufferLen {
		return newDbError(
			"unexpected data at the end of the data section (last offset: %v, end: %v)",
			offset,
			bufferLen,
		)
	}

	if len(offsets) != 0 {
		return newDbError(
			"found %v pointers (of %v) in the search tree that we did not see in the data section",
			len(offsets),
			pointerCount,
		)
	}
	return nil
}
