package reader

import (
	"fmt"
	"github.com/globalmac/idx"
	"math/big"
	"os"
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
			t.Error("Ошибка извлечения файла БД:", err)
		}
		os.Remove("./../test.db.enc")
	}

	dbr, err := Open(cfn)
	if err != nil {
		t.Error("Ошибка чтения БД:", err)
	}
	defer dbr.Close()

	fmt.Println("=== Данные о БД ===")
	fmt.Println("Дата создания:", time.Unix(int64(dbr.Metadata.BuildEpoch), 0).Format("2006-01-02 в 15:01:05"), "Кол-во узлов:", dbr.Metadata.NodeCount, "Кол-во данных:", dbr.Metadata.Total)

	var Record struct {
		ID   uint64 `idx:"id"`
		Data struct {
			Detail struct {
				ID      uint64  `idx:"id"`
				Val     string  `idx:"val"`
				Bool    bool    `idx:"bool"`
				Double  float64 `idx:"double"`
				Float   float32 `idx:"float"`
				Uint128 big.Int `idx:"uint128"`
				Uint16  uint16  `idx:"uint16"`
				Uint32  uint32  `idx:"uint16"`
				Uint64  uint64  `idx:"uint16"`
				Utf8    string  `idx:"utf8"`
			} `idx:"detail"`
		} `idx:"data"`
		Value string         `idx:"value"`
		Slice []any          `idx:"slice"`
		Map   map[string]any `idx:"map"`
	}

	///

	/*fmt.Println("=== Поиск по ID  ===")

	var id uint64 = 398257
	result := dbr.Find(id)

	if result.Exist() {
		_ = result.Decode(&Record)
		fmt.Println("Запись:", Record.ID, Record.Value, Record.Data.Detail.ID) //Record.Slice, Record.Map
	} else {
		fmt.Printf("Запись c ID = %d не найдена!\n\r", id)
	}*/

	/*fmt.Println("=== Проход по диапазону (С 1 и ПО 5 запись) ===")

	for row := range dbr.GetRange(1, 3) {
		if row.Exist() {
			_ = row.Decode(&Record)
			fmt.Println(Record.ID, Record.Value, Record.Slice, Record.Map)
		}
	}*/

	// STRING
	result1 := "Привет 12345!"
	//[]any{"data", "detail", "val"}, "Ключ-123323"
	dbr.Where([]any{"value"}, "=", result1, func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if result1 == Record.Value {
				fmt.Printf("Тест 1 / Найдена запись с ID = %d / Value = %s\n", Record.ID, Record.Value)
			} else {
				t.Errorf("Тест 1 провален, результат: %s, ожидалось: %s.", Record.Value, result1)
			}
			return false
		} else {
			t.Errorf("Тест 1 провален, результат: %d, ожидалось: %s.", 0, result1)
		}
		return true
	})

	dbr.Where([]any{"map", "item_1", "value"}, "=", "Счастье10000", func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if uint64(10000) == Record.ID {
				fmt.Printf("Тест 1.1 / Найдена запись с ID = %d / Map = %v\n", Record.ID, Record.Map)
			} else {
				t.Errorf("Тест 1.1 провален, результат: %d, ожидалось: %v.", Record.ID, 10000)
			}
			return false
		} else {
			t.Errorf("Тест 1.1 провален, результат: %d, ожидалось: %v.", 0, 10000)
		}
		return true
	})

	dbr.Where([]any{"data", "detail", "utf8"}, "=", "unicode777!😀", func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if uint64(777) == Record.ID {
				fmt.Printf("Тест 1.2 / Найдена запись с ID = %d / Map = %v\n", Record.ID, Record.Map)
			} else {
				t.Errorf("Тест 1.2 провален, результат: %d, ожидалось: %v.", Record.ID, 777)
			}
			return false
		} else {
			t.Errorf("Тест 1.2 провален, результат: %d, ожидалось: %v.", 0, 777)
		}
		return true
	})

	dbr.Where([]any{"value"}, "LIKE", "ривет 100!", func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if Record.Value == "Привет 100!" {
				fmt.Printf("Тест 2 / Найдена запись с ID = %d / Value = %s\n", Record.ID, Record.Value)
			} else {
				t.Errorf("Тест 2 провален, результат: %s, ожидалось: %s.", Record.Value, "Привет 100!")
			}
			return false
		} else {
			t.Errorf("Тест 2 провален, результат: %d, ожидалось: %s.", 0, "Привет 100!")
		}
		return true
	})

	dbr.Where([]any{"value"}, "ILIKE", "риВЕТ 777", func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if Record.Value == "Привет 777!" {
				fmt.Printf("Тест 3 / Найдена запись с ID = %d / Value = %s\n", Record.ID, Record.Value)
			} else {
				t.Errorf("Тест 3 провален, результат: %s, ожидалось: %s.", Record.Value, "Привет 777!")
			}
			return false
		} else {
			t.Errorf("Тест 3 провален, результат: %d, ожидалось: %s.", 0, "Привет 777!")
		}
		return true
	})

	// FLOAT
	result2 := float64(42.1)
	dbr.Where([]any{"data", "detail", "double"}, "=", result2, func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if result2 == Record.Data.Detail.Double {
				fmt.Printf("Тест 4 / Найдена запись с ID = %d / Data.Detail.Double = %f\n", Record.ID, Record.Data.Detail.Double)
			} else {
				t.Errorf("Тест 4 провален, результат: %f, ожидалось: %f.", Record.Data.Detail.Double, result2)
			}
			return false
		} else {
			t.Errorf("Тест 4 провален, результат: %d, ожидалось: %f.", 0, result2)
		}
		return true
	})

	result3 := float32(23.335)
	dbr.Where([]any{"data", "detail", "float"}, "=", result3, func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if result3 == Record.Data.Detail.Float {
				fmt.Printf("Тест 5 / Найдена запись с ID = %d / Data.Detail.Float = %v\n", Record.ID, Record.Data.Detail.Float)
			} else {
				t.Errorf("Тест 5 провален, результат: %f, ожидалось: %v.", Record.Data.Detail.Float, result3)
			}
			return false
		} else {
			t.Errorf("Тест 5 провален, результат: %d, ожидалось: %v.", 0, result3)
		}
		return true
	})

	// BOOL
	dbr.Where([]any{"data", "detail", "bool"}, "=", true, func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if true == Record.Data.Detail.Bool {
				fmt.Printf("Тест 6 / Найдена запись с ID = %d / Data.Detail.Bool = %t\n", Record.ID, Record.Data.Detail.Bool)
			} else {
				t.Errorf("Тест 6 провален, результат: %t, ожидалось: %t.", Record.Data.Detail.Bool, true)
			}
			return false
		} else {
			t.Errorf("Тест 6 провален, результат: %d, ожидалось: %t.", 0, true)
		}
		return true
	})

	dbr.Where([]any{"data", "detail", "bool"}, "=", false, func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if false == Record.Data.Detail.Bool {
				fmt.Printf("Тест 7 / Найдена запись с ID = %d / Data.Detail.Bool = %t\n", Record.ID, Record.Data.Detail.Bool)
			} else {
				t.Errorf("Тест 7 провален, результат: %t, ожидалось: %t.", Record.Data.Detail.Bool, false)
			}
			return false
		} else {
			t.Errorf("Тест 7 провален, результат: %d, ожидалось: %t.", 0, false)
		}
		return true
	})

	// INT
	result4 := int(1000000)
	dbr.Where([]any{"map", "item_3", "id"}, "=", result4, func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if uint64(result4-1) == Record.ID { // Так надо
				fmt.Printf("Тест 8 / Найдена запись с ID = %d\n", Record.ID)
			} else {
				t.Errorf("Тест 8 провален, результат: %d, ожидалось: %d.", Record.ID, result4)
			}
			return false
		} else {
			t.Errorf("Тест 8 провален, результат: %d, ожидалось: %d.", 0, result4)
		}
		return true
	})

	dbr.Where([]any{"map", "item_3", "id"}, "<", result4, func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if uint64(1) == Record.ID {
				fmt.Printf("Тест 9 / Найдена запись с ID = %d\n", Record.ID)
			} else {
				t.Errorf("Тест 9 провален, результат: %d, ожидалось: %d.", Record.ID, 1)
			}
			return false
		} else {
			t.Errorf("Тест 9 провален, результат: %d, ожидалось: %d.", 0, 1)
		}
		return true
	})

	dbr.Where([]any{"map", "item_3", "id"}, ">", result4, func(result Result) bool {
		var tr = uint64(result4 + 1)
		if err := result.Decode(&Record); err == nil {
			if Record.Map["item_3"].(map[string]any)["id"] == tr { // так надо
				fmt.Printf("Тест 10 / Найдена запись с ID = %v, Value = %v\n", Record.ID, Record.Map["item_3"].(map[string]any)["id"])
			} else {
				t.Errorf("Тест 10 провален, результат: %v, ожидалось: %d.", Record.Map["item_3"].(map[string]any)["id"], tr)
			}
			return false
		} else {
			t.Errorf("Тест 10 провален, результат: %v, ожидалось: %d.", 0, tr)
		}
		return true
	})

	dbr.Where([]any{"id"}, "!=", 1, func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if uint64(2) == Record.ID {
				fmt.Printf("Тест 11 / Найдена запись с ID = %v\n", Record.ID)
			} else {
				t.Errorf("Тест 11 провален, результат: %v, ожидалось: %d.", Record.ID, 2)
			}
			return false
		} else {
			t.Errorf("Тест 11 провален, результат: %v, ожидалось: %d.", 0, 2)
		}
		return true
	})

	// BYTE + SLICE
	dbr.Where([]any{"slice", 1}, "=", []byte{1, 2, 3, 4}, func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if uint64(1) == Record.ID {
				fmt.Printf("Тест 12 / Найдена запись с ID = %v, Value = %v\n", Record.ID, Record.Slice[1])
			} else {
				t.Errorf("Тест 12 провален, результат: %v, ожидалось: %d.", Record.ID, 1)
			}
			return false
		} else {
			t.Errorf("Тест 12 провален, результат: %v, ожидалось: %d.", 0, 1)
		}
		return true
	})

	//UINT16/32/64

	dbr.Where([]any{"data", "detail", "uint16"}, "=", uint16(16), func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if 1 == Record.ID {
				fmt.Printf("Тест 13 / Найдена запись с ID = %v, Value = %v\n", Record.ID, Record.Data.Detail.Uint16)
			} else {
				t.Errorf("Тест 13 провален, результат: %v, ожидалось: %d.", Record.Data.Detail.Uint16, 1)
			}
			return false
		} else {
			t.Errorf("Тест 13 провален, результат: %v, ожидалось: %d.", 0, 1)
		}
		return true
	})

	dbr.Where([]any{"data", "detail", "uint32"}, "<", uint32(32), func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if 1001 == Record.ID {
				fmt.Printf("Тест 14 / Найдена запись с ID = %v\n", Record.ID)
			} else {
				t.Errorf("Тест 14 провален, результат: %v, ожидалось: %d.", Record.ID, 1001)
			}
			return false
		} else {
			t.Errorf("Тест 14 провален, результат: %v, ожидалось: %d.", 0, 1001)
		}
		return true
	})

	dbr.Where([]any{"data", "detail", "uint64"}, ">", uint64(3), func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if 1 == Record.ID {
				fmt.Printf("Тест 15 / Найдена запись с ID = %v\n", Record.ID)
			} else {
				t.Errorf("Тест 15 провален, результат: %v, ожидалось: %d.", Record.ID, 1)
			}
			return false
		} else {
			t.Errorf("Тест 15 провален, результат: %v, ожидалось: %d.", 0, 1)
		}
		return true
	})

	//SLICE
	dbr.Where([]any{"slice", 0}, "=", "Привет слайс1000000!", func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if uint64(1000000) == Record.ID {
				fmt.Printf("Тест 16 / Найдена запись с ID = %v\n", Record.ID)
			} else {
				t.Errorf("Тест 16 провален, результат: %v, ожидалось: %d.", Record.ID, 1000000)
			}
			return false
		} else {
			t.Errorf("Тест 16 провален, результат: %v, ожидалось: %d.", 0, 1000000)
		}
		return true
	})

	// BIG INT

	bigInt := big.Int{}
	bigInt.SetString("18446744073709551615777123", 10)

	dbr.Where([]any{"data", "detail", "uint128"}, "=", &bigInt, func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			if 123 == Record.ID {
				fmt.Printf("Тест 17 / Найдена запись с ID = %v\n", Record.ID)
			} else {
				t.Errorf("Тест 17 провален, результат: %v, ожидалось: %d.", Record.ID, 123)
			}
			return false
		} else {
			t.Errorf("Тест 17 провален, результат: %v, ожидалось: %d.", 0, 123)
		}
		return true
	})

	// []IN
	var c = []string{"Ключ-10", "Ключ-555", "Ключ-900000"}
	var u = 0
	dbr.Where([]any{"data", "detail", "val"}, "IN", c, func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			u++
			if Record.Data.Detail.Val == "Ключ-10" || Record.Data.Detail.Val == "Ключ-555" || Record.Data.Detail.Val == "Ключ-900000" {
				fmt.Printf("Тест 18 / Найдена запись с ID = %v, Value = %v\n", Record.ID, Record.Data.Detail.Val)
			} else {
				t.Errorf("Тест 18 провален, результат: %v, ожидалось: %v.", Record.Data.Detail.Val, c)
			}
			if len(c) == u {
				return false
			}
			return true
		} else {
			t.Errorf("Тест 18 провален, результат: %v, ожидалось: %v.", 0, c)
		}
		return true
	})

	var c2 = []int{123, 77777, 510777}
	var u2 = 0
	dbr.Where([]any{"map", "item_3", "id"}, "IN", c2, func(result Result) bool {
		if err := result.Decode(&Record); err == nil {
			u2++
			var td = Record.Map["item_3"].(map[string]any)["id"]
			if td == uint64(123) || td == uint64(77777) || td == uint64(510777) {
				fmt.Printf("Тест 19 / Найдена запись с ID = %v, Value = %v\n", Record.ID, td)
			} else {
				t.Errorf("Тест 19 провален, результат: %v, ожидалось: %v.", td, c2)
			}
			if len(c2) == u2 {
				return false
			}
			return true
		} else {
			t.Errorf("Тест 19 провален, результат: %v, ожидалось: %v.", 0, c2)
		}
		return true
	})

	t.Log("Все ОК")

}
