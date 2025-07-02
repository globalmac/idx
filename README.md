# IDX

Пилотный проект бинарной встраиваемой поисковой базы данных KEY => VALUE на Golang для очень быстрого поиска по числовым ключам.

> Важно: в активной разработке, скоро добавлю больше примеров использования и покрою тестами

### Зачем и для чего использовать:

- Идеально подходит для формирования поисковой БД с числовыми ключами, например для номера телефона или числового представления строкового значения из БД
- Фантастически быстрая работа поиска с хорошо сбалансированным деревом и узлами: мгновенный доступ по ключу и диапазонам
- Минимальное использования ОЗУ
- Удобные типы данных для структур JSON-like с максимальной производительностью на чтение
- Есть пример шифрования и сжатия файла БД с чувствительными данными (~ в 4-5 раз уменьшит размер файла)

Пример из жизни:

У нас есть своя база клиентов: где ключ = (uint64) номер телефона 70001112233 и значения в виде мапы, например:

При записи в БД (writer.Insert(70001112233, json)) у нас будет:

`Ключ = 70001112233`

И значение из структуры, например:

```json 
{
  "id": 123,
  "name": "Джуна",
  "surname": "Симонс",
  "patronymic": "",
  "sex": 2,
  "geo": {
    "lat": "11.11",
    "lon": "22.2233"
  },
  "active": true, 
  "options": {
    "character": "positive",
    "smile": 1
  },
  "slice": [
    "1",
    2,
    3
  ]
}
```

Чтобы мгновенно найти по ключу - используем метод `reader.Find(70001112233)`.

Чтобы найти в значении `name = "Борис"` (медленнее чем по ключу и кушает ресурсы, если БД большая) - используем `reader.Where([]any{"name"}, "=", "Борис", func(result Result) bool {})`

Немного позже будет реализован функционал с текстовыми ключами, но в числовом представлении - поиск по ним будет очень быстрый.

### Особенности:

- Без сторонних зависимостей и библиотек
- Используется B-tree с UINT64 ключём и значениями в виде Slice и Map (см. ниже примеры)
- Очень высокая скорость работы с минимальным использованием памяти на чтение
- Mmap (отображение файлов в память)
- Функциональный сериализатор/десериализатор данных для значений (JSON-like)
- Состоит из writer - создает индекс (файл БД) и reader - читает файл
- Функции поиска по ID (Find), итерация по всему файлу (GetAll), поиск значения в мапе/слайсе и т.д. (Where), выборка по диапазону (Range с 1 по 5, например)
- При поиске по значения (WHERE) поддерживается производительный поиск: "=", "!=", "<", ">", "IN", "LIKE", "ILIKE".
- Сжимает (Tar GZ), хеширует (Murmur3) и шифрует (AES-256) данные и файлы БД (при использовании функций EncryptDB/DecryptDB)

> Вдохновение и общая идея + сериализатор/десериализатор взяты из формата данных MMDB (MaxMind Database) и в частности: https://github.com/maxmind/mmdbwriter (writer) и https://github.com/oschwald/maxminddb-golang (reader) для поиска по IP-адресам

### Типы данных значений:

- Map
- Slice
- Bytes
- String
- Bool
- Uint16/32/64/128
- Int32
- Float32/64

Все из них можно комбинировать между собой и хранить средние и большие структуры данных.

### Минусы:

- За ключами (Uint64) необходимо следить самостоятельно (в случае повтора - данные перезапишутся)
- Ресурсоёмкий процесс формирования файлов БД - данные пишутся буфером в память и затем записывается в файл на диск. При индексации больших объёмов данных, лучше делить его на небольшие партиции по 1-10 млн записей.

## Установка:

```
go get -u github.com/globalmac/idx
```

## Примеры использования

Для начала необходимо создать индекс на основании Ваших данных. 

Этот процесс при большом кол-ве записей будет изрядно тратить ОЗУ, но оно того стоит. 

Если у Вас не очень много ОЗУ на устройстве - лучше делать маленькие партиции данных и затем через индекс составить их карту партиций (позже будет описание с примерами).

Далее Вы можете использовать производительные функции чтения:

- **Find** - найдет узел по ID
- **GetAll** - вернет все узлы
- **GetRange(start, end)** - вернет диапазон "с" и "по" записей
- **Where** - поиск по значениям (внутри структуры данных значений)

### Writer: создание индекса (файла БД)

Готовим 1000 записей и записываем их в файл.

Внутри представлен закомментированный блок с шифрованием данных и их сжатием после записи в файл.

```golang
package main

import (
	"fmt"
	"github.com/globalmac/idx/reader"
	"github.com/globalmac/idx/writer"
	"os"
	"strconv"
)

func main() {

	var filename = "test.db"

	// Открываем файл для записи
	dbFile, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer dbFile.Close()

	// Инициируем writer для записи нового индекса БД
	db, err := writer.New(
		writer.Config{
			Name: "Название БД",
		},
	)
	if err != nil {
		panic(err)
	}

	// Формируем 1000 записей
	var i uint64
	for i = 1; i <= 1000; i++ {

		strID := strconv.Itoa(int(i))

		// Структуру значения
		var record = writer.DataMap{
			"id":    writer.DataUint64(i),
			"value": writer.DataString("Привет это значение - " + strID),
			"slice": writer.DataSlice{
				writer.DataString("слайс строка " + strID),
				writer.DataUint64(1),
				writer.DataUint64(2),
				writer.DataUint64(3),
			},
			"map": writer.DataMap{
				"item_1": writer.DataMap{
					"id":    writer.DataUint16(1),
					"value": writer.DataString("Счастье"),
				},
				"item_2": writer.DataMap{
					"id":    writer.DataUint16(2),
					"value": writer.DataString("Счастье 2"),
				},
				"item_3": writer.DataMap{
					"id":    writer.DataUint16(3),
					"value": writer.DataString("Счастье 3"),
				},
			},
		}

		// Скидываем в буфер
		err = db.Insert(i, record)
		if err != nil {
			fmt.Println(err)
		}

		// Поиск после записи - просто для примера
		//row, r := db.Find(i)
		//fmt.Println("--- Поиск по дереву в моменте:", row, r)

	}

	// Сериализация и запись данных из буфера в файл
	_, err = db.Serialize(dbFile)
	if err != nil {
		panic(err)
	}

	// Пример шифрования и сжатия записанного файла - опционально
	/*err = idx.EncryptDB(filename, filename+".enc", "SecretPwd123")
	if err != nil {
		fmt.Println("Ошибка шифрования и архивации файл БД:", err)
		return
	}
	// Удаляем файл с чистовыми данными, оставляя только сжатый шифрованный .enc
	os.Remove(filename)*/

}

```

### Reader: чтение индекса (файла БД)

Поиск, итератор по всем значениям, выборка диапазона, поиск внутри структуры.

Внутри представлен закомментированный блок с дешифрованием данных и декомпрессией перед открытием файла.

```go
package main

import (
	"fmt"
	"github.com/globalmac/idx/reader"
	"syscall"
	"time"
)

func main() {

	var filename = "test.db"

	// Пример дешифрования и декомпрессии записанного .enc файла - опционально
	/*
	// Для UNIX - проверяем есть ли чистовой файл 
	if syscall.Stat(filename, &syscall.Stat_t{}) != nil {
	    // Извлекаем и расшифровываем test.db.enc и сохраняем его как test.db
		err := idx.DecryptDB(filename+".enc", filename, "SecretPwd123")
		if err != nil {
			fmt.Println("Ошибка извлечения файла БД:", err)
			return
		}
	    // Опционально - удаляем шифрованный архив, так как у нас есть чистовые данные
	    //os.Remove(filename+".enc")
	}*/

	// Открываем файл для чтения
	dbr, err := reader.Open(filename)
	if err != nil {
		panic(err)
	}
	defer dbr.Close()

	fmt.Println("=== Мета-данные о БД ===")
	
	fmt.Println(
		"Дата создания БД:", time.Unix(int64(dbr.Metadata.BuildEpoch), 0).Format("2006-01-02 в 15:01:05"),
		"Кол-во узлов:", dbr.Metadata.NodeCount,
		"Кол-во всех данных:", dbr.Metadata.Total,
	)

	// Структура данных
	var Record struct {
		ID    uint64         `idx:"id"`
		Value string         `idx:"value"`
		Slice []any          `idx:"slice"`
		Map   map[string]any `idx:"map"`
	}

	///

	fmt.Println("=== Поиск по ID  ===")

	var id uint64 = 50

	result := dbr.Find(id)

	if result.Exist() {
		_ = result.Decode(&Record)
		fmt.Println("Запись:", Record.ID, Record.Value, Record.Slice, Record.Map)
	} else {
		fmt.Printf("Запись c ID = %d не найдена!\n\r", id)
	}

	///

	fmt.Println("=== Проход по всем записям ===")

	for row := range dbr.GetAll() {
		if row.Exist() {
			_ = row.Decode(&Record)
			fmt.Println(Record.ID, Record.Value, Record.Slice, Record.Map)
		}
	}

	///

	fmt.Println("=== Проход по диапазону (С 1 и ПО 5 запись) ===")

	for row := range dbr.GetRange(1, 5) {
		if row.Exist() {
			_ = row.Decode(&Record)
			fmt.Println(Record.ID, Record.Value, Record.Slice, Record.Map)
		}
	}

	///

	fmt.Println("=== Поиск в значении Record.Value ===")

	dbr.Where([]any{"value"}, "=", "Привет это значение - 25", func(result reader.Result) bool {
		if err = result.Decode(&Record); err == nil {
			fmt.Println("Найдена запись:", Record.ID, Record.Value, Record.Slice, Record.Map)
			return false // Если нужно вернуть первое вхождение, иначе вернет все найденные записи
		}
		return true
	})

}

```

### Reader: поиск внутри значения (метод Where)

Удобный, расширенный и быстрый поиск внутри структуры значений.

Поддерживаются операции: "=", "!=", "<", ">", "IN", "LIKE", "ILIKE".

```go
package main

import (
	"fmt"
	"github.com/globalmac/idx/reader"
	"syscall"
	"time"
)

func main() {

	var filename = "test.db"

	// Пример дешифрования и декомпрессии записанного .enc файла - опционально
	/*
	// Для UNIX - проверяем есть ли чистовой файл 
	if syscall.Stat(filename, &syscall.Stat_t{}) != nil {
	    // Извлекаем и расшифровываем test.db.enc и сохраняем его как test.db
		err := idx.DecryptDB(filename+".enc", filename, "SecretPwd123")
		if err != nil {
			fmt.Println("Ошибка извлечения файла БД:", err)
			return
		}
	    // Опционально - удаляем шифрованный архив, так как у нас есть чистовые данные
	    //os.Remove(filename+".enc")
	}*/

	// Открываем файл для чтения
	dbr, err := reader.Open(filename)
	if err != nil {
		panic(err)
	}
	defer dbr.Close()
	
	/* Памятка по значениям при записи
    var record = writer.DataMap{
        "id":    writer.DataUint64(i),
        "value": writer.DataString("Привет это значение - "+strID),
        "slice": writer.DataSlice{
            writer.DataString("слайс строка "+strID),
            writer.DataUint64(1),
            writer.DataUint64(2),
            writer.DataUint64(3),
        },
        "map": writer.DataMap{
            "item_1": writer.DataMap{
                "id":    writer.DataUint16(1),
                "value": writer.DataString("Счастье"),
            },
            "item_2": writer.DataMap{
                "id":    writer.DataUint16(2),
                "value": writer.DataString("Счастье 2"),
            },
            "item_3": writer.DataMap{
                "id":    writer.DataUint16(3),
                "value": writer.DataString("Счастье 3"),
            },
        },
    }*/
	
	// Структура данных
	var Record struct {
		ID    uint64         `idx:"id"`
		Value string         `idx:"value"`
		Slice []any          `idx:"slice"`
		Map   map[string]any `idx:"map"`
	}
	
	///

	fmt.Println("=== Поиск ключу в значении ===")

	// Для string можно использовать: "=", "!=", "IN", "LIKE", "ILIKE"
	dbr.Where([]any{"value"}, "LIKE", "это значение - 25", func(result reader.Result) bool {
		if err = result.Decode(&Record); err == nil {
			fmt.Println("Найдена запись:", Record.ID, Record.Value, Record.Slice, Record.Map)
			return false // Если нужно вернуть первое вхождение, иначе вернет все найденные записи
		}
		return true
	})

	var values = []string{"Ключ-10", "Ключ-555", "Ключ-900"}
	var counter = 0
	// Найдём все values
	dbr.Where([]any{"value"}, "IN", values, func(result reader.Result) bool {
		if err = result.Decode(&Record); err == nil {
			counter++ // Увеличиваем счетчик вхождений
			fmt.Println("Найдена запись:", Record.ID, Record.Value, Record.Slice, Record.Map)
			if len(values) == counter {
				return false // Если счетчик найденных = кол-ву values - останавливаемся
			}
			return true
		}
		return true
	})

}

```

Еще примеры поиска:

```go
// В строке
dbr.Where([]any{"value"}, "ILIKE", "привет", func(result reader.Result) bool {})

// В мапе
dbr.Where([]any{"map", "item_3", "id"}, "=", 1, func(result reader.Result) bool {})

// В мапе по ключу
dbr.Where([]any{"map", "items", 2, "row"}, "=", 100, func(result reader.Result) bool {})

// В слайсе по ключу 0 (DataString)
dbr.Where([]any{"slice", 0}, ">", 3, func(result reader.Result) bool {})

// В слайсе по ключу 2 (DataUint64)
dbr.Where([]any{"slice", 2}, "<", 3, func(result reader.Result) bool {})

// IN - поддерживает []string, []int и []uint64
dbr.Where([]any{"value"},  "IN", []string{"Привет", "Текст", "Выход"}, func(result reader.Result) bool {})
dbr.Where([]any{"id"},     "IN", []int{111, 77777, 510777}, func(result reader.Result) bool {})
dbr.Where([]any{"big_id"}, "IN", []uint64int{111123, 77777000, 510777000}, func(result reader.Result) bool {})

```

Больше примеров поиска в `/reader/read_test.go => TestReadFileSecure()`
