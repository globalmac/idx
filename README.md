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

var i uint64
for i = 1; i <= 10; i++ {

    var record = writer.DataMap{
        "id":    writer.DataUint64(i),
        "value": writer.DataString("Привет!"),
        "slice": writer.DataSlice{
            writer.DataString("строка 1"),
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
    
    err = db.Insert(i, record)
    if err != nil {
        fmt.Println(err)
    }
    
    row, r := db.Find(i)
    fmt.Println("--- Поиск по дереву:", row, r)

}

_, err = db.Serialize(dbFile)
if err != nil {
    panic(err)
}

```

### Чтение индекса (файла БД)

```go

dbr, err := reader.Open("test.db")
if err != nil {
    panic(err)
}
defer dbr.Close()

fmt.Println("=== Данные о БД ===")
fmt.Println("Дата создания:", time.Unix(int64(dbr.Metadata.BuildEpoch), 0).Format("2006-01-02 в 15:01:05"), "Кол-во узлов:", dbr.Metadata.NodeCount)

var Record struct {
    ID    uint64         `idx:"id"`
    Value string         `idx:"value"`
    Slice []string       `idx:"slice"`
    Map   map[string]any `idx:"map"`
}

///

fmt.Println("=== Поиск по ID  ===")

result := dbr.Find(1)

if result.Exist() {
    _ = result.Decode(&Record)
    fmt.Println("Запись:", Record.ID, Record.Value, Record.Slice, Record.Map)
} else {
	fmt.Printf("Запись c ID = %d не найдена!\n\r", id)
}

///

fmt.Println("=== Проход по всем записям ===")

for net := range dbr.GetAll() {
    if row.Exist() {
        _ = row.Decode(&Record)
        fmt.Println(Record.ID, Record.Value, Record.Slice, Record.Map)
    }
}

///

fmt.Println("=== Проход с лимитами по периоду (С 1 и ПО 5 запись) ===")

for row := range dbr.GetRange(1, 5) {
    if row.Exist() {
        _ = row.Decode(&Record)
        fmt.Println(Record.ID, Record.Value, Record.Slice, Record.Map)
    }
}

///

fmt.Println("=== Поиск ключу в значении (медленный) ===")

dbr.Where("value", "Привет!", func(result reader.Result) bool {
    if err = result.Decode(&Record); err == nil {
        fmt.Println("Найдена запись:", Record.ID, Record.Value, Record.Slice, Record.Map)
        return false // Если нужно вернуть первое вхождение, иначе вернет все найденные записи
    }
    return true
})


```