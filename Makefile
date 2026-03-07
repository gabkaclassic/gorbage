.PHONY: build test proto help \
	up down docker-build gorbage redis minio \
	cipher-key \
	certs server-certs client-certs clean-certs clean-server-certs clean-client-certs verify-certs show-certs-config \
	

DAYS ?= 3650
DNS ?= localhost
IP ?= 127.0.0.1
CLIENT ?= gorbage-client
CA_CN ?= gorbage-ca
SERVER_CN ?= gorbage-server
SERVER_DIR = config/server
CLIENT_DIR = config/client
CA_KEY = $(SERVER_DIR)/ca.key
CA_CRT = $(SERVER_DIR)/ca.crt
SERVER_KEY = $(SERVER_DIR)/server.key
SERVER_CSR = server.csr
SERVER_CRT = $(SERVER_DIR)/server.crt
SERVER_EXT = server.ext
CLIENT_KEY = $(CLIENT_DIR)/client.key
CLIENT_CSR = client.csr
CLIENT_CRT = $(CLIENT_DIR)/client.crt
CLIENT_EXT = client.ext
CA_SERIAL = $(SERVER_DIR)/ca.srl

CIPHER_KEY_LENGTH ?= 32
CIPHER_KEY_PATH ?= config/client/cipher

COMPOSE_FILE ?= config/docker/docker-compose.yml

build:
	go build -o build/server cmd/server/main.go
	go build -o build/client cmd/client/main.go

cipher-key:
	@openssl rand -base64 48 | head -c $(CIPHER_KEY_LENGTH) > $(CIPHER_KEY_PATH)

up: 
	docker compose -f ${COMPOSE_FILE} up -d

docker-build: 
	docker compose -f ${COMPOSE_FILE} build gorbage

down: 
	docker compose -f ${COMPOSE_FILE} down

gorbage:
	docker compose -f ${COMPOSE_FILE} up gorbage -d

redis:
	docker compose -f ${COMPOSE_FILE} up redis -d

minio:
	docker compose -f ${COMPOSE_FILE} up minio -d

proto:
	protoc \
		--proto_path=api/proto \
		--go_out=internal/proto \
		--go_opt=paths=source_relative \
		--go-grpc_out=internal/proto \
		--go-grpc_opt=paths=source_relative \
		api/proto/gorbage.proto

certs: server-certs client-certs

server-certs: | $(SERVER_DIR)
	@echo "=== Generating CA certificate ==="
	openssl genrsa -out $(CA_KEY) 4096
	openssl req -x509 -new -nodes \
		-key $(CA_KEY) \
		-sha256 -days $(DAYS) \
		-out $(CA_CRT) \
		-subj "/CN=$(CA_CN)"

	@echo "=== Generating server certificate ==="
	openssl genrsa -out $(SERVER_KEY) 4096
	openssl req -new \
		-key $(SERVER_KEY) \
		-out $(SERVER_CSR) \
		-subj "/CN=$(SERVER_CN)"

	@echo "subjectAltName=DNS:$(DNS),IP:$(IP)" > $(SERVER_EXT)
	@echo "extendedKeyUsage=serverAuth" >> $(SERVER_EXT)

	openssl x509 -req \
		-in $(SERVER_CSR) \
		-CA $(CA_CRT) \
		-CAkey $(CA_KEY) \
		-CAcreateserial \
		-out $(SERVER_CRT) \
		-days $(DAYS) \
		-sha256 \
		-extfile $(SERVER_EXT)

client-certs: server-certs | $(CLIENT_DIR)
	openssl genrsa -out $(CLIENT_KEY) 4096
	openssl req -new \
		-key $(CLIENT_KEY) \
		-out $(CLIENT_CSR) \
		-subj "/CN=$(CLIENT)"

	@echo "extendedKeyUsage=clientAuth" > $(CLIENT_EXT)

	openssl x509 -req \
		-in $(CLIENT_CSR) \
		-CA $(CA_CRT) \
		-CAkey $(CA_KEY) \
		-CAcreateserial \
		-out $(CLIENT_CRT) \
		-days $(DAYS) \
		-sha256 \
		-extfile $(CLIENT_EXT)

	cp $(CA_CRT) $(CLIENT_DIR)/ca.crt


$(SERVER_DIR):
	mkdir -p $(SERVER_DIR)

$(CLIENT_DIR):
	mkdir -p $(CLIENT_DIR)

clean-certs:
	@echo "=== Cleaning up certificates ==="
	rm -f $(SERVER_CSR) $(CLIENT_CSR) $(SERVER_EXT) $(CLIENT_EXT) $(CA_SERIAL)
	rm -rf $(SERVER_DIR) $(CLIENT_DIR)
	@echo "Cleanup completed"

clean-server-certs:
	rm -f $(SERVER_CSR) $(SERVER_EXT) $(CA_SERIAL)
	rm -rf $(SERVER_DIR)

clean-client-certs:
	rm -f $(CLIENT_CSR) $(CLIENT_EXT)
	rm -rf $(CLIENT_DIR)

show-certs-config:
	@echo "=== Current configuration ==="
	@echo "DAYS:           $(DAYS)"
	@echo "DNS:            $(DNS)"
	@echo "IP:             $(IP)"
	@echo "CLIENT:         $(CLIENT)"
	@echo "CA_CN:          $(CA_CN)"
	@echo "SERVER_CN:      $(SERVER_CN)"
	@echo "Server dir:     $(SERVER_DIR)"
	@echo "Client dir:     $(CLIENT_DIR)"

verify-certs:
	@echo "=== Verifying server certificate ==="
	openssl verify -CAfile $(CA_CRT) $(SERVER_CRT)
	@echo "=== Verifying client certificate ==="
	openssl verify -CAfile $(CA_CRT) $(CLIENT_CRT)