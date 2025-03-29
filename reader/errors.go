package reader

import (
	"fmt"
	"reflect"
)

// DbError указывает, что БД содержит недопустимые данные и не может быть проанализирована должным образом
type DbError struct {
	reason string
}

// newOffsetError возвращает новую ошибку БД в непредвиденных случаях c EOF
func newOffsetError() DbError {
	return DbError{"неожиданный EOF БД"}
}

// newDbError возвращает новую ошибку повреждения БД
func newDbError(format string, args ...interface{}) DbError {
	return DbError{fmt.Sprintf(format, args...)}
}

// Error реализует интерфейс ошибки
func (e DbError) Error() string {
	return e.reason
}

// TypeMismatchError возникает, когда значение БД не может быть присвоено целевому типу
type TypeMismatchError struct {
	ExpectedType reflect.Type
	ActualValue  string
}

// newUnmarshalStrError выводит ошибку несоответствия типа из строкового значения
func newUnmarshalStrError(value string, typ reflect.Type) TypeMismatchError {
	return TypeMismatchError{
		ExpectedType: typ,
		ActualValue:  value,
	}
}

// newUnmarshalError выводит TypeMismatchError из произвольного значения
func newUnmarshalError(value interface{}, typ reflect.Type) TypeMismatchError {
	return newUnmarshalStrError(fmt.Sprintf("%v (%T)", value, value), typ)
}

// Error реализует интерфейс ошибки с подробной информацией о несоответствии типов
func (e TypeMismatchError) Error() string {
	return fmt.Sprintf("idx: нельзя присвоить %s типу %s", e.ActualValue, e.ExpectedType)
}
