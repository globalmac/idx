# IDX

Пилотный проект бинарной встраиваемой поисковой базы данных KEY => VALUE на Golang для очень быстрого поиска по числовым ключам.

> Важно: в активной разработке, скоро добавлю больше примеров использования и покрою тестами

### Зачем и для чего использовать:

- Идеально подходит для формирования поисковой БД с числовыми ключами, например для номера телефона или числового представления строкового значения из БД
- Фантастически быстрая работа поиска с хорошо сбалансированным деревом и узлами: мгновенный доступ по ключу и диапазонам
- Минимальное использования ОЗУ
- Удобные типы данных для структур JSON-like с максимальной производительностью на чтение
- Есть пример шифрования и сжатия файла БД с чувствительными данными (~ в 4-5 раз уменьшит размер файла)

### Особенности:

- Без сторонних зависимостей и библиотек
- Используется B-tree с UINT64 ключём и значениями в виде Slice и Map (см. ниже примеры)
- Очень высокая скорость работы с минимальным использованием памяти на чтение
- Mmap (отображение файлов в память)
- Функциональный сериализатор/десериализатор данных для значений (JSON-like)
- Состоит из writer - создает индекс (файл БД) и reader - читает файл
- Функции поиска по ID (Find), итерация по всему файлу (GetAll), поиск значения в мапе (Where), выборка по диапазону (Range с 1 по 5, например)
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

### Создание индекса (файла БД)

Готовим 1000 записей и записываем их в файл.

Внутри представлен закомментированный блок с шифрованием данных и их сжатием после записи в файл.

```golang
package main

import (
	"fmt"
	"github.com/globalmac/idx/reader"
	"github.com/globalmac/idx/writer"
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
			"value": writer.DataString("Привет это значение - "+strID),
			"slice": writer.DataSlice{
				writer.DataString("слайс строка "+strID),
				writer.DataUint64(1),
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
		row, r := db.Find(i)
		fmt.Println("--- Поиск по дереву в моменте:", row, r)

	}

	// Сериализация и запись данных из буфера в файл
	_, err = db.Serialize(dbFile)
	if err != nil {
		panic(err)
	}	
	
	// Пример шифрования и сжатия записанного файла - опционально
	/*err := idx.EncryptDB(filename, filename+".enc", "SecretPwd123")
	if err != nil {
		fmt.Println("Ошибка шифрования и архивации файл БД:", err)
		return
	}
	// Удаляем файл с чистовыми данными, оставляя только сжатый шифрованный .enc
	os.Remove(filename)*/
	
}

```

### Чтение индекса (файла БД)

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
		Slice []string       `idx:"slice"`
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

	fmt.Println("=== Поиск ключу в значении ===")

	dbr.Where("value", "Привет это значение - 25", func(result reader.Result) bool {
		if err = result.Decode(&Record); err == nil {
			fmt.Println("Найдена запись:", Record.ID, Record.Value, Record.Slice, Record.Map)
			return false // Если нужно вернуть первое вхождение, иначе вернет все найденные записи
		}
		return true
	})

}

```