package writer

import (
	"fmt"
	"github.com/globalmac/idx/encrypt"
	"math/big"
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

		var bigInt = big.Int{}
		bigInt.SetString("18446744073709551615777"+str, 10)
		uint128 := DataUint128(bigInt)
		//
		var floatStr64, _ = strconv.ParseFloat("42."+str, 64)
		var floatStr32, _ = strconv.ParseFloat("23."+str, 32)

		var u16 = 1
		var u32 = 2
		var u64 = 3

		var bv = false
		if i >= 1 && i <= 1000 {
			bv = true
			u16 = 16
			u32 = 32
			u64 = 64
		}

		var record = DataMap{
			"id":          DataUint64(i),
			"value":       DataString("Привет " + str + "!"),
			"empty_value": DataString(""),
			"empty_id":    DataUint64(0),
			"data": DataMap{
				"detail": DataMap{
					"id":      DataUint64(i),
					"val":     DataString("Ключ-" + str),
					"bool":    DataBool(bv),
					"double":  DataFloat64(floatStr64),
					"float":   DataFloat32(float32(floatStr32)),
					"uint128": &uint128,
					"uint16":  DataUint16(u16),
					"uint32":  DataUint32(u32),
					"uint64":  DataUint64(u64),
					"utf8":    DataString("unicode" + str + "!😀"),
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
					"value": DataString("Счастье" + str),
				},
				"item_2": DataMap{
					"id":    DataUint16(2),
					"value": DataString("Счастье 2"),
				},
				"item_3": DataMap{
					"id":    DataUint64(i + 1),
					"value": DataString("Счастье 3"),
				},
			},
		}

		db.Insert(i, record)
		//db.InsertDefaultNull(i, record)

	}

	db.Serialize(dbFile)

	err := encrypt.EncryptDB(fn, "./../test.db.enc", "0ih7iDiipucs9AqNOHf")
	if err != nil {
		fmt.Println("Ошибка шифрования и архивации файл БД:", err)
		return
	}

	os.Remove(fn)

}

func TestCreateFileAndInsertDefaultNull(t *testing.T) {

	var fn = "./../test3.db"
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
			"id":          DataUint64(i),
			"value":       DataString("Привет " + str + "!"),
			"empty_value": DataString(""),
			"empty_id":    DataUint64(0),
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

		db.InsertDefaultNull(i, record)

	}

	db.Serialize(dbFile)

	//os.Remove(fn)

}

func TestCreateFileWithID(t *testing.T) {

	var fn = "./../test4.db"
	dbFile, _ := os.Create(fn)
	defer dbFile.Close()

	db, _ := New(
		Config{
			Name: "БД",
		},
	)

	var i uint64
	for i = 1; i <= 1_000_000; i++ {

		var str = strconv.Itoa(int(i))

		//var record = DataString("Привет " + str + "!")
		var record = DataSlice{
			DataString("Привет " + str + "!"),
			DataUint64(i),
		}

		db.InsertDefaultNull(i, record)

	}

	db.Serialize(dbFile)

	//os.Remove(fn)

}

func TestCreatePartitionFile(t *testing.T) {

	ids := []uint64{
		1, 2, 3,
		4, 5,
		10, 15,
		100, 150, 151,
		1000, 1001, 5000,
	}

	// 3 партиции
	parts := 3
	ranges := CreatePartitions(ids, parts)

	// Берём все партиции
	for _, r := range ranges {

		fmt.Println(r.Part)

		var pn = strconv.Itoa(int(r.Part))

		var fn = "./../part_" + pn + ".db"
		dbFile, _ := os.Create(fn)
		defer dbFile.Close()

		var db, _ = New(
			Config{
				Name: "БД с партициями",
				Partitions: &PartitionsConfig{
					Current: r.Part,
					Total:   uint64(parts),
					Ranges:  ranges,
				},
			},
		)

		// Берём все ID
		for _, id := range ids {

			var partition = GetPartition(id, ranges)

			if partition == r.Part {

				var str = strconv.Itoa(int(id))

				db.Insert(id, DataSlice{
					DataString("Привет " + str + "!"),
					DataUint64(id),
					DataUint64(partition),
				})

				fmt.Println("---", id, partition)

			}

		}

		db.Serialize(dbFile)

	}

	//os.Remove(fn)

}
