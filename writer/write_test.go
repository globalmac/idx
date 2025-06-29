package writer

import (
	"fmt"
	"github.com/globalmac/idx"
	"os"
	"strconv"
	"testing"
)

func TestCreateFileAndInsert(t *testing.T) {

	var fn = "./../test.db"
	dbFile, _ := os.Create(fn)
	defer dbFile.Close()

	db, _ := New(
		Config{
			Name: "Название БД",
		},
	)

	var i uint64
	for i = 1; i <= 10_000_000; i++ {

		var str = strconv.Itoa(int(i))

		var record = DataMap{
			"id":    DataUint64(i),
			"value": DataString("Привет " + str + "!"),
			"slice": DataSlice{
				//DataUint64(1),
				DataString("Привет слайс" + str + "!"),
				DataBytes{1, 2, 3, 4},
				DataUint64(i),
			},
			"map": DataMap{
				"item_1": DataMap{
					"id":    DataUint16(1),
					"value": DataString("Счастье"),
				},
				"item_2": DataMap{
					"id":    DataUint16(2),
					"value": DataString("Счастье 2"),
				},
				"item_3": DataMap{
					"id":    DataUint16(3),
					"value": DataString("Счастье 3"),
				},
			},
		}

		db.Insert(i, record)

	}

	db.Serialize(dbFile)

	//os.Remove(fn)

}

func TestCreateFileAndInsertSecure(t *testing.T) {

	var fn = "./../test2.db"
	dbFile, _ := os.Create(fn)
	defer dbFile.Close()

	db, _ := New(
		Config{
			Name: "Название БД",
		},
	)

	var i uint64
	for i = 1; i <= 10_000_000; i++ {

		var str = strconv.Itoa(int(i))

		var record = DataMap{
			"id":    DataUint64(i),
			"value": DataString("Привет " + str + "!"),
			"data": DataMap{
				"detail": DataMap{
					"id": DataUint64(i),
				},
			},
			"slice": DataSlice{
				DataString("Привет слайс" + str + "!"),
				DataBytes{1, 2, 3, 4},
				DataUint64(i),
			},
			"map": DataMap{
				"item_1": DataMap{
					"id":    DataUint16(1),
					"value": DataString("Счастье"),
				},
				"item_2": DataMap{
					"id":    DataUint16(2),
					"value": DataString("Счастье 2"),
				},
				"item_3": DataMap{
					"id":    DataUint16(3),
					"value": DataString("Счастье 3"),
				},
			},
		}

		db.Insert(i, record)

	}

	db.Serialize(dbFile)

	err := idx.EncryptDB(fn, "./../test.db.enc", "0ih7iDiipucs9AqNOHf")
	if err != nil {
		fmt.Println("Ошибка шифрования и архивации файл БД:", err)
		return
	}

	os.Remove(fn)

}
