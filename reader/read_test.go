package reader

import (
	"fmt"
	"github.com/globalmac/idx"
	"syscall"
	"testing"
	"time"
)

func TestReadFile(t *testing.T) {

	var fn = "./../test.db"
	dbr, _ := Open(fn)
	defer dbr.Close()

	fmt.Println("=== Данные о БД ===")
	fmt.Println("Дата создания:", time.Unix(int64(dbr.Metadata.BuildEpoch), 0).Format("2006-01-02 в 15:01:05"), "Кол-во узлов:", dbr.Metadata.NodeCount, "Кол-во данных:", dbr.Metadata.Total)

	var Record struct {
		ID    uint64         `idx:"id"`
		Value string         `idx:"value"`
		Slice []any          `idx:"slice"`
		Map   map[string]any `idx:"map"`
	}

	///

	fmt.Println("=== Поиск по ID  ===")

	var id uint64 = 999_000
	result := dbr.Find(id)

	if result.Exist() {
		_ = result.Decode(&Record)
		fmt.Println("Запись:", Record.ID, Record.Value, Record.Slice, Record.Map)
	} else {
		fmt.Printf("Запись c ID = %d не найдена!\n\r", id)
	}

	/*fmt.Println("=== Проход с лимитами по периоду (С 1 и ПО 5 запись) ===")

	for row := range dbr.GetRange(1, 5) {
		if row.Exist() {
			_ = row.Decode(&Record)
			fmt.Println(Record.ID, Record.Value, Record.Slice, Record.Map)
		}
	}*/

	///

	/*fmt.Println("=== Поиск ключу в значении (медленный) ===")

	dbr.Where("value", "Привет!", func(result reader.Result) bool {
		if err = result.Decode(&Record); err == nil {
			fmt.Println("Найдена запись:", Record.ID, Record.Value, Record.Slice, Record.Map)
			return false // Если нужно вернуть первое вхождение, иначе вернет все найденные записи
		}
		return true
	})*/

}

func TestReadFileSecure(t *testing.T) {

	var cfn = "./../test2.db"

	if syscall.Stat(cfn, &syscall.Stat_t{}) != nil {
		err := idx.DecryptDB("./../test.db.enc", cfn, "0ih7iDiipucs9AqNOHf")
		if err != nil {
			fmt.Println("Ошибка извлечения файла БД:", err)
			return
		}
		//os.Remove("./../test.db.enc")
	}

	dbr, _ := Open(cfn)
	defer dbr.Close()

	fmt.Println("=== Данные о БД ===")
	fmt.Println("Дата создания:", time.Unix(int64(dbr.Metadata.BuildEpoch), 0).Format("2006-01-02 в 15:01:05"), "Кол-во узлов:", dbr.Metadata.NodeCount, "Кол-во данных:", dbr.Metadata.Total)

	var Record struct {
		ID   uint64 `idx:"id"`
		Data struct {
			Detail struct {
				ID uint64 `idx:"id"`
			} `idx:"detail"`
		} `idx:"data"`
		Value string         `idx:"value"`
		Slice []any          `idx:"slice"`
		Map   map[string]any `idx:"map"`
	}

	///

	fmt.Println("=== Поиск по ID  ===")

	var id uint64 = 3298257
	result := dbr.Find(id)

	if result.Exist() {
		_ = result.Decode(&Record)
		fmt.Println("Запись:", Record.ID, Record.Value, Record.Slice, Record.Map)
	} else {
		fmt.Printf("Запись c ID = %d не найдена!\n\r", id)
	}

	/*fmt.Println("=== Проход по диапазону (С 1 и ПО 5 запись) ===")

	for row := range dbr.GetRange(1, 3) {
		if row.Exist() {
			_ = row.Decode(&Record)
			fmt.Println(Record.ID, Record.Value, Record.Slice, Record.Map)
		}
	}

	dbr.Where("value", "Привет 1!", func(result Result) bool {
		//dbr.Where2("id", uint16(112), func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			fmt.Println("Найдена запись:", Record.ID, Record.Value, Record.Slice, Record.Map, Record.Data.Detail.ID)
			return true // Если нужно вернуть первое вхождение, иначе вернет все найденные записи
		}
		return true
	})*/

}
