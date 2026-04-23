# Auto Monitor - gRPC & WebSocket

Sistema de monitoramento em tempo real para máquinas Windows e Linux.

## Requisitos

- [Go](https://go.dev/doc/install) (v1.18+)
- [Protocol Buffers Compiler (protoc)](https://grpc.io/docs/protoc-installation/)
- Plugins Go para protoc:
  ```bash
  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
  ```

## Estrutura do Projeto

- `/proto`: Definição do contrato gRPC.
- `/server`: Servidor que recebe métricas gRPC, atualiza o Supabase e distribui via WebSocket.
- `/client`: Agente coletor de métricas (CPU, RAM, Disco, Rede).

## Configuração

1. Gere os arquivos gRPC a partir do proto:
   ```bash
   protoc --go_out=. --go_opt=paths=source_relative \
          --go-grpc_out=. --go-grpc_opt=paths=source_relative \
          proto/monitor.proto
   ```

2. Configure o arquivo `.env` dentro da pasta `server/` com suas credenciais do Supabase e do InfluxDB.

## InfluxDB e Retenção de Dados

O InfluxDB é usado para persistir o histórico de métricas. Para garantir que os dados com mais de 30 dias sejam removidos automaticamente, você deve configurar a **Retention Policy** no bucket:

1. Ao criar o bucket via UI do InfluxDB, selecione "Set Retention" para 30 dias.
2. Ou via CLI:
   ```bash
   influx bucket update --name monitor-bucket --retention 720h
   ```

## Como Executar

### Servidor
```bash
cd server
go run main.go
```
*   gRPC: porta 50051
*   WebSocket: porta 8080 (endpoint `/ws`)

### Cliente (Agente)
```bash
cd client
go run main.go
```
*O cliente solicitará o `code_app` (licença) ao iniciar.*

## Compilação para Produção (Executáveis)

Para gerar os binários para Windows e Linux:

### Linux
```bash
# Servidor
GOOS=linux GOARCH=amd64 go build -o monitor-server-linux ./server
# Cliente
GOOS=linux GOARCH=amd64 go build -o monitor-client-linux ./client
```

### Windows (.exe)
```bash
# Servidor
GOOS=windows GOARCH=amd64 go build -o monitor-server.exe ./server
# Cliente
GOOS=windows GOARCH=amd64 go build -o monitor-client.exe ./client
```

## Integração com a Web

O dashboard web deve se conectar ao servidor via WebSocket:
`ws://<ip-do-servidor>:8080/ws`

O servidor enviará um JSON a cada segundo para cada máquina ativa:
```json
{
  "code_app": "LICENSE-123",
  "cpu_usage": 15.5,
  "ram_usage": 60.2,
  "disk_usage": 45.0,
  "network_tx": 1024.5,
  "network_rx": 2048.1
}
```
