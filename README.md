# IDX

Пилотный проект бинарной встраиваемой поисковой базы данных KEY => VALUE на Golang для очень быстрого поиска по числовым ключа.

> Важно: в активной разработке, использовать только для тестов + скоро добавлю больше примеров использования

### Особенности: 

- Без сторонних зависимостей
- Используется B-tree с UINT64 ключём и значениями в виде Slice и Map (см. ниже примеры)
- Очень высокая скорость работы с минимальным использованием памяти на чтение
- Mmap (отображение файлов в память)
- Функциональный сериализатор/десериализатор данных для значений
- Состоит из writer - создает индекс (файл БД) и reader - читает файл
- Функции поиска по ID (Find), итерация по всему файлу (GetAll), поиск значения в мапе (Where), выборка по диапазону (Range с 1 по 5, например)

> Вдохновение и общая идея + сериализатор/десериализатор взяты из формата данных MMDB (MaxMind Database) и в частности: https://github.com/maxmind/mmdbwriter (writer) и https://github.com/oschwald/maxminddb-golang (reader) для поиска по IP-адресам

### Зачем и для чего использовать:

- Идеально подходит для формирования поисковой БД с числовыми ключами, например для номера телефона или числовом представлении строкового значения
- Очень удобные типы данных значений с максимальной производительностью на чтение
- Можно партиционировать файлы БД (например, по 1 млн записей в файл) и объединять их при поиске

### Типы данных значений:

- Map
- Slice
- Bytes
- String
- Bool
- Uint16/32/64/128
- Int32
- Float32/64

### Минусы:

- За ключами (Uint64) необходимо следить самостоятельно (в случае повтора - данные перезапишутся)
- Долгий и ресурсоёмкий процесс формирования файлов БД при индексации больших объёмов данных (лучше делить его на небольшие партиции по 1 млн записей)
- Файлы БД не шифруются, а хешируются алгоритмом Murmur3 для производительности (будьте аккуратны с чувствительными данными)

## Установка:

```
go get -u github.com/globalmac/idx
```

## Примеры использования


### Создание индекса (файла БД)

```golang

dbFile, err := os.Create("test.db")
if err != nil {
    panic(err)
}
defer dbFile.Close()

db, err := writer.New(
    writer.Config{
        Name: "Название БД",
    },
)
if err != nil {
    panic(err)
}

record := writer.DataMap{
    "id":     writer.DataUint64(123),
    "value":  writer.DataString("Привет!"),
}

err = db.Insert(123123, record)
if err != nil {
    panic(err)
}

```

### Чтение индекса (файла БД)

```go

db, err := reader.Open("test.db")
if err != nil {
    panic(err)
}
defer db.Close()

var Record struct {
    ID     uint64 `idx:"id"`
    Value  string `idx:"value"`
}

result := db.Find(id)

if result.Exist() {
    _ = result.Decode(&Record)
    fmt.Println(Record.ID, Record.Value)
}

```

### Комбинированный пример

- createIdx - cоздание индекса (файла БД) на основании рандомных данных
- find - поиск по ID

```go

package main

import (
	"fmt"
	"idx/reader"
	"idx/writer"
	"log"
	"math/rand"
	"net"
	"os"
	"runtime"
	"runtime/debug"
	"time"
)

const (
	lineLength = 10
	charset    = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

func randomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func randomBool() bool {
	return seededRand.Intn(2) == 1
}

func randomIP() string {
	return fmt.Sprintf("%d.%d.%d.%d/%s",
		seededRand.Intn(256), // Генерация числа от 0 до 255
		seededRand.Intn(256),
		seededRand.Intn(256),
		seededRand.Intn(256),
		"24",
	)
}

func createIdx() {
	dbFile, err := os.Create("test.db")
	if err != nil {
		fmt.Println(err)
	}
	defer dbFile.Close()

	db, err := writer.New(
		writer.Config{
			Name: "БД",
		},
	)
	if err != nil {
		panic(err)
	}

	for i := 1; i < 100_000; i++ {

		var IP = randomIP()
		_, network, _ := net.ParseCIDR(IP)
		rndStr := randomString(lineLength)
		bigInt := big.Int{}
		bigInt.SetString("1329227995784915872903807060280344576", 10)
		uint128 := writer.DataUint128(bigInt)

		record := writer.DataMap{
			"id":    writer.DataUint64(i),
			"name":  writer.DataString("Привет! Строка = " + rndStr),
			"ip":    writer.DataString(network.IP.String()),
			"value": writer.DataString(rndStr),
			"bool":  writer.DataBool(randomBool()),
			"slice": writer.DataSlice{
				writer.DataString("строка 1"),
				writer.DataString("строка 2"),
				writer.DataString("строка 3"),
				writer.DataUint64(1),
				writer.DataUint64(2),
				writer.DataUint64(3),
			},
			"bytes": writer.DataBytes{
				0x0,
				0x0,
				0x0,
				0x2a,
			},
			"double": writer.DataFloat64(42.123456),
			"float":  writer.DataFloat32(1.1),
			"int32":  writer.DataInt32(-268435456),
			"map": writer.DataMap{
				"item_1": writer.DataMap{
					"x": writer.DataSlice{
						writer.DataUint64(0x7),
						writer.DataUint64(0x8),
						writer.DataUint64(0x9),
					},
					"value": writer.DataString("Счастье"),
				},
				"item_2": writer.DataMap{
					"x": writer.DataSlice{
						writer.DataUint64(0x7),
						writer.DataUint64(0x8),
						writer.DataUint64(0x9),
					},
					"value": writer.DataString("Счастье 2"),
				},
				"item_3": writer.DataMap{
					"x": writer.DataSlice{
						writer.DataUint64(0x7),
						writer.DataUint64(0x8),
						writer.DataUint64(0x9),
					},
					"value": writer.DataString("Счастье 2"),
				},
			},
			"uint128":     &uint128,
			"uint16":      writer.DataUint64(0x64),
			"uint32":      writer.DataUint64(0x10000000),
			"uint64":      writer.DataUint64(0x1000000000000000),
			"utf8_string": writer.DataString("unicode! ☯ - ♫"),
		}

		err = db.Insert(i, record)
		if err != nil {
			fmt.Println(err)
		}

		// Выводим ID
		fmt.Println(i)

		// Проверяем какие данные будут записаны
		prefixLen, r := db.Find(i)
		fmt.Println("---", prefixLen, r)

	}

	// Сериализуем данные и записываем в файл
	_, err = db.Serialize(dbFile)
	if err != nil {
		panic(err)
	}
}

func main() {

	// Снижение пикового потребления памяти для запуска Garbage Collector
	os.Setenv("GOGC", "30")
	debug.SetGCPercent(30)

	// Замеряем начальное использование памяти
	var startMemStats runtime.MemStats
	runtime.ReadMemStats(&startMemStats)
	// Замеряем время начала операции
	startTime := time.Now()

	//===

	createIdx()

	//===

	// Замеряем время окончания операции
	elapsedTime := time.Since(startTime)
	// Замеряем конечное использование памяти
	var endMemStats runtime.MemStats
	runtime.ReadMemStats(&endMemStats)
	// Вычисляем использование памяти
	memoryUsed := endMemStats.Alloc - startMemStats.Alloc

	// Выводим результаты
	fmt.Println("-------")
	fmt.Printf("Время: %v\n", elapsedTime)
	fmt.Printf("Оперативка: %d байт (%.2f MB)", memoryUsed, float64(memoryUsed)/1024/1024)

}
```




