# Назначение

Клиент/сервер для приёма артефактов (файлы/директории) по gRPC. Клиент — CLI, шифрует и сжимает данные локально; сервер — принимает по mTLS и кладёт в хранилище.

# Стек

* Go — код сервера и клиента.
* gRPC — транспорт.
* MinIO — целевое хранилище.
* Redis — очередь/метаданные.
* Docker — dev/prod окружение.
* OpenSSL — генерация mTLS сертификатов.

# Быстрый старт

* Сборка: `make build` — собирает `build/server` и `build/client`.
* Протоколы: `make proto` (генерация `internal/proto`).
* Контейнеры: `make up` / `make down` (использует `config/docker/docker-compose.yml`).
* Сертификаты: `make certs` (CA → сервер → клиент; по умолчанию в `config/server` и `config/client`).

# Запуск

* Сервер: `./build/server --address localhost:8080 --cert config/server/server.crt --key config/server/server.key --ca config/server/ca.crt --minio-host localhost:9000 ...`
* Клиент (пример): `./build/client push /path/to/dir --server localhost:8080 --cert config/client/client.crt --key config/client/client.key --ca config/server/ca.crt --cipher-key config/client/cipher`

# Примеры использования клиента (3 примера)

1. Push директории:
   `./build/client push /path/to/dir --prefix projects/proj1 --cipher-key config/client/cipher`
2. Pull файла:
   `./build/client pull projects/proj1/file.tar.gz /tmp/out --cipher-key config/client/cipher`
3. Список по префиксу:
   `./build/client ls projects/proj1 --server localhost:8080`

# Конфигурация

* Файлы конфигурации/сертификаты по умолчанию: `config/server`, `config/client`.
* Docker compose: `config/docker/docker-compose.yml`.
* Основные переменные окружения/флаги:

  * Minio: `MINIO_HOST`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_USE_SSL`.
  * Redis: `REDIS_HOST`, `REDIS_PASSWORD`, `REDIS_USER`, `REDIS_DB`.
  * GRPC/TLS: `--address`, `--name`, `--key`, `--cert`, `--ca`.
  * Клиент: флаги `push|pull|ls|inf|del`, `--cipher-key`, `--workers`, `--chunk-size`.

# Протокол

* .proto: `api/proto/gorbage.proto`.
* Генерация: `protoc --proto_path=api/proto --go_out=internal/proto --go-grpc_out=internal/proto api/proto/gorbage.proto`.
* Сервер ожидает mTLS; клиент предъявляет клиентский сертификат.

# Модель работы

* Клиент:

  * локально сжимает и шифрует (ключ хранится в `--cipher-key`), формирует части (chunk).
  * инициирует загрузку к серверу по gRPC и шлёт части.
* Сервер:

  * проверяет mTLS; извлекает bucket-name из client certificate (CN) / политики.
  * записывает полученные объекты в `MinIO`.
  * использует `Redis` для TTL/очередей и метаданных.

# Безопасность

* Шифрование данных выполняется на клиенте; сервер не хранит незашифрованные данные.
* mTLS обязателен: сервер валидирует клиент через CA (`--ca`).
* Сервер доверяет CIDR из `TrustedCIDRs` (config).

# Тесты и покрытие

* `make test` — запуск тестов, генерация покрытия `coverage.out` и вывод функции покрытия.
* Отчёт фильтрует автоген и protobuf файлы.

# Makefile — полезные таргеты

* `build`, `test`, `proto`, `certs`, `server-certs`, `client-certs`, `verify-certs`, `clean-certs`, `up`, `down`, `docker-build`, `minio`, `redis`.

# Файловая структура (ключевые пути)

* `cmd/server` — entrypoint сервера.
* `cmd/client` — entrypoint клиента (CLI).
* `internal/proto` — сгенерированные pb/.pb.go.
* `api/proto/gorbage.proto` — исходный proto.
* `config/docker` — compose и .env примеры.
* `config/server`, `config/client` — сертификаты и конфиги по умолчанию.

# Контакты командных утилит

* Протоколы и сборка автоматизированы через Makefile; для нестандартных окружений — менять флаги и env.
