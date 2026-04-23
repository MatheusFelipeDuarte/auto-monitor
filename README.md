# 🚀 Auto Monitor - Observability Suite

Sistema profissional de monitoramento em tempo real para infraestruturas híbridas (Windows/Linux), escalável via Docker e com interface desktop moderna.

---

## 🛠️ Tecnologias
- **Backend Core**: Go (gRPC + WebSockets)
- **Persistência**: InfluxDB (Séries temporais) + Supabase (Gerenciamento de Licenças)
- **Visualização**: Grafana (Advanced) + Embedded Dashboard (Live)
- **Desktop Client**: Wails (Go + React)
- **Infra**: Docker & Docker Compose

---

## 📂 Estrutura do Projeto
- `/proto`: Contrato de comunicação gRPC.
- `/server`: Servidor centralizado e dashboard web embutido.
- `/client-desktop`: Nova interface desktop premium (Wails).
- `/client`: Agente coletor legidado (CLI).
- `/grafana`: Configurações e dashboards pré-provisionados.

---

## 🏗️ Como Executar o Servidor (Docker Stack)

O servidor agora utiliza uma stack completa de observabilidade.

1.  **Pré-requisitos**: Docker e Docker Compose instalados.
2.  **Configuração**: Edite o arquivo `.env` na raiz do projeto com suas chaves do Supabase.
3.  **Iniciar**:
    ```bash
    docker-compose up --build
    ```

### 🛰️ Acessando os Dashboards
- **Live Dashboard (Go Native)**: [http://localhost:8080](http://localhost:8080)
- **Grafana (Histórico & Analytics)**: [http://localhost:3000](http://localhost:3000)
    - *Usuário/Senha padrão: `admin` / `admin`*

---

## 🖥️ Como Executar o Cliente Desktop (GUI)

O novo cliente desktop oferece uma experiência moderna para o usuário final.

1.  **Pré-requisitos**: Instale o [Wails CLI](https://wails.io/docs/gettingstarted/installation).
2.  **Iniciar em Modo Dev**:
    ```bash
    cd client-desktop
    wails dev
    ```
3.  **Compilar Executável**:
    ```bash
    wails build
    ```

---

## ⚠️ Solução de Problemas

### Conflito de Portas
Se ao rodar o `docker-compose` você receber um erro como:
`Bind for 0.0.0.0:8086 failed: port is already allocated`

Significa que você já tem o InfluxDB (ou outro serviço) rodando nessa porta. Para resolver:
1.  Pare o serviço local: `sudo systemctl stop influxdb` (se estiver no Linux).
2.  Ou altere a porta no `docker-compose.yml` na seção de `ports` do serviço `influxdb`.

### gRPC Protobuf
Se precisar alterar o contrato de comunicação:
```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       proto/monitor.proto
```

---

## 📊 Formato dos Dados (Real-time)
O servidor distribui via WebSocket o seguinte payload JSON:
```json
{
  "code_app": "NOME-DA-MAQUINA",
  "cpu_usage": 15.5,
  "ram_usage": 60.2,
  "disk_usage": 45.0,
  "network_tx": 1024.5,
  "network_rx": 2048.1
}
```
