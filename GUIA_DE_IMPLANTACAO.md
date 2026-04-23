# Guia de Implantação e Migração para Produção

Este guia descreve o que você precisa fazer para tirar o sistema (Server, Auto-Tools e Client) do seu "localhost" e colocá-lo para rodar no ambiente de produção (sua VPS / Servidor Cloud / Vercel).

---

## 1. O Pipeline (Visão Geral Simplificada)

A estrutura do seu sistema em produção ficará assim:

1. **VPS (Servidor Linux na Nuvem)**
   Vai hospedar o seu back-end (o Docker Compose do `auto-monitor`). 
   - Recebe as conexões via gRPC.
   - Guarda os dados no InfluxDB.

2. **Vercel ou mesma VPS (Auto-Tools / Painel Web SaaS)**
   O seu Painel SaaS (`auto-tools`). 
   - Acessa o banco de dados (Supabase) via internet.

3. **Máquinas dos Clientes**
   Vão rodar o `auto-monitor-client` (Telinha Wails ou Agent Silencioso) compilado para **apontar para o IP da sua VPS**.

---

## 2. O Que Mudar em Cada Arquivo e Servidor

Antes de fazer o "Deploy", você precisa trocar os endereços `localhost` pelos endereços oficiais da sua VPS ou domínio de produção.

### 2.1 Backend / Servidor (`auto-monitor`) -> Vai para a VPS

**O que precisa ajustar:**
1. Variáveis de Ambiente e Arquivo `.env`:
   - Enviar a pasta `server`, `grafana`, `influxdb` e o `docker-compose.yml` para a VPS.
   - Criar o `.env` na VPS mantendo as variáveis do seu Supabase (`SUPABASE_URL` e `SUPABASE_KEY`).
2. Na VPS, portas devem estar liberadas no Firewall da sua operadora (AWS, DO, Contabo):
   - **Porta `50051` (TCP)**: Tem que estar aberta para o MUNDO. É onde seus clientes (agentes) irão conectar para mandar as métricas de saúde da máquina.
   - Restante (`8080` de logs e `3001` do grafana): Apenas abertas para os seus IPs ou fechadas por trás de um proxy (NGINX+HTTPS).

### 2.2 Client Desktop (`auto-monitor/client-desktop`) -> Vai para as máquinas dos clientes

Antes de compilar o aplicativo para enviar para os clientes, você tem que ajustar o código dele:

**O que precisa ajustar (Obrigatório!):**
1. Abra o arquivo: `auto-monitor/client-desktop/app.go`
2. Modifique a linha ~54 com o IP estático da VPS ou o domínio público da sua API que fará as conexões do App do seu cliente com o seu Server:
   - De: `conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))`
   - Para: `conn, err := grpc.Dial("IP_DA_SUA_VPS:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))`

*(Esse client desktop será compilado gerando um EXE ou binário nas etapas do MANUAL_INSTALACAO_CLIENT.md e entregue para cliente via download / email).*

### 2.3 Dashboard Web (`auto-tools`) -> Vai para a Vercel 

Este é o React (seu painel). Sendo Vercel, o Vercel lida com a infraestrutura.

**O que precisa ajustar:**
1. Apenas adicionar no painel Environment Variables do dashboard da Vercel todas as ENV config do seu `.env.local` que você utiliza hoje no auto-tools. 
2. Caso o auto-tools for precisar escutar algo direto do servidor monitor via websockets futuramente, o host deverá ser o `http://IP_DA_SUA_VPS:8080`.
