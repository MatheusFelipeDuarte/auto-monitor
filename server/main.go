package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	pb "auto-monitor/proto"

	"github.com/gorilla/websocket"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/joho/godotenv"
	"github.com/supabase-community/supabase-go"
	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedMonitorServiceServer
	supabaseClient *supabase.Client
	influxClient   influxdb2.Client
	influxWriteAPI api.WriteAPI
	mu             sync.RWMutex
	clients        map[string]*MachineData
	wsClients      map[*websocket.Conn]bool
	logClients     map[*websocket.Conn]bool
	activeStreams  map[string]context.CancelFunc
}

// Global log broadcaster
var logChan = make(chan string, 100)

type logWriter struct {
	mu     sync.Mutex
	stdout io.Writer
	curr   string
	file   *os.File
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if date changed
	date := time.Now().Format("02-01-2006")
	if date != w.curr {
		if w.file != nil {
			w.file.Close()
		}
		filename := fmt.Sprintf("server-%s.log", date)
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Printf("Erro ao abrir arquivo de log %s: %v\n", filename, err)
			return 0, err
		}
		w.file = f
		w.curr = date
	}

	msg := string(p)
	select {
	case logChan <- msg:
	default:
	}

	n, err = w.file.Write(p)
	w.stdout.Write(p)
	return n, err
}

type MachineData struct {
	CodeApp   string  `json:"code_app"`
	CPUUsage  float64 `json:"cpu_usage"`
	RAMUsage  float64 `json:"ram_usage"`
	DiskUsage float64 `json:"disk_usage"`
	NetworkTX float64 `json:"network_tx"`
	NetworkRX float64 `json:"network_rx"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *server) Authenticate(ctx context.Context, req *pb.AuthRequest) (*pb.AuthResponse, error) {
	log.Printf("Tentativa de autenticação: %s", req.CodeApp)
	
	// Verificar se a licença existe
	var results []map[string]interface{}
	_, err := s.supabaseClient.From("licenses_monitor").Select("*", "exact", false).Eq("code_app", req.CodeApp).ExecuteTo(&results)
	
	if err != nil || len(results) == 0 {
		log.Printf("Licença inválida ou não encontrada: %s", req.CodeApp)
		return &pb.AuthResponse{Success: false, Message: "Licença inválida ou não autorizada para monitoramento."}, nil
	}

	// Licença válida, atualizar flag de conexão
	data := map[string]interface{}{"is_connected": true}
	_, _, err = s.supabaseClient.From("licenses_monitor").Update(data, "representation", "minimal").Eq("code_app", req.CodeApp).Execute()
	
	if err != nil {
		return &pb.AuthResponse{Success: false, Message: "Erro ao atualizar status no banco: " + err.Error()}, nil
	}

	log.Printf("✅ Cliente autenticado e conectado: %s", req.CodeApp)
	return &pb.AuthResponse{Success: true, Message: "Conectado"}, nil
}

func (s *server) Disconnect(ctx context.Context, req *pb.DisconnectRequest) (*pb.DisconnectResponse, error) {
	log.Printf("Disconnect request for: %s", req.CodeApp)
	
	data := map[string]interface{}{"is_connected": false}
	_, _, err := s.supabaseClient.From("licenses_monitor").Update(data, "representation", "minimal").Eq("code_app", req.CodeApp).Execute()
	
	s.mu.Lock()
	delete(s.clients, req.CodeApp)
	s.mu.Unlock()

	if err != nil {
		return &pb.DisconnectResponse{Success: false}, nil
	}
	return &pb.DisconnectResponse{Success: true}, nil
}

func (s *server) TransmitMetrics(stream pb.MonitorService_TransmitMetricsServer) error {
	var currentClientCode string
	
	// Criar um contexto que podemos cancelar externamente
	ctx, cancel := context.WithCancel(stream.Context())
	
	defer func() {
		cancel()
		if currentClientCode != "" {
			log.Printf("Conexão encerrada para: %s", currentClientCode)
			s.mu.Lock()
			delete(s.clients, currentClientCode)
			delete(s.activeStreams, currentClientCode)
			s.mu.Unlock()
			
			statusOffline := map[string]interface{}{"is_connected": false}
			s.supabaseClient.From("licenses_monitor").Update(statusOffline, "representation", "minimal").Eq("code_app", currentClientCode).Execute()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("conexão encerrada pelo servidor (licença removida ou inválida)")
		default:
			req, err := stream.Recv()
			if err != nil {
				return err
			}
			
			if currentClientCode == "" {
				currentClientCode = req.CodeApp
				s.mu.Lock()
				// Se já existia uma conexão para este código, cancelamos a anterior
				if oldCancel, exists := s.activeStreams[currentClientCode]; exists {
					oldCancel()
				}
				s.activeStreams[currentClientCode] = cancel
				s.mu.Unlock()
			}

			data := &MachineData{
				CodeApp:   req.CodeApp,
				CPUUsage:  req.CpuUsage,
				RAMUsage:  req.RamUsage,
				DiskUsage: req.DiskUsage,
				NetworkTX: req.NetworkTx,
				NetworkRX: req.NetworkRx,
			}

			s.mu.Lock()
			s.clients[req.CodeApp] = data
			s.mu.Unlock()

			log.Printf("📊 Metrics from %s: CPU=%.1f%% RAM=%.1f%%", req.CodeApp, req.CpuUsage, req.RamUsage)
			s.broadcastToWS(data)
			s.saveToInflux(data)
		}
	}
}

func (s *server) saveToInflux(data *MachineData) {
	if s.influxWriteAPI == nil {
		return
	}
	p := influxdb2.NewPoint("system_metrics",
		map[string]string{"code_app": data.CodeApp},
		map[string]interface{}{
			"cpu_usage":  data.CPUUsage,
			"ram_usage":  data.RAMUsage,
			"disk_usage": data.DiskUsage,
			"network_tx": data.NetworkTX,
			"network_rx": data.NetworkRX,
		},
		time.Now())
	s.influxWriteAPI.WritePoint(p)
}

func (s *server) watchLicenses() {
	for {
		time.Sleep(10 * time.Second) // Validação a cada 10s (pode ser ajustado)
		
		s.mu.RLock()
		if len(s.activeStreams) == 0 {
			s.mu.RUnlock()
			continue
		}
		
		var activeCodes []string
		for code := range s.activeStreams {
			activeCodes = append(activeCodes, code)
		}
		s.mu.RUnlock()

		// Busca em lote no Supabase para ser eficiente
		var dbLicenses []map[string]interface{}
		_, err := s.supabaseClient.From("licenses_monitor").Select("code_app, is_active", "exact", false).In("code_app", activeCodes).ExecuteTo(&dbLicenses)
		
		if err != nil {
			log.Printf("Erro ao validar licenças no Watcher: %v", err)
			continue
		}

		// Mapear quem ainda é válido
		validOnes := make(map[string]bool)
		for _, lic := range dbLicenses {
			if isActive, ok := lic["is_active"].(bool); ok && isActive {
				if code, ok := lic["code_app"].(string); ok {
					validOnes[code] = true
				}
			}
		}

		// Expulsar quem não está mais no banco ou foi desativado
		s.mu.Lock()
		for code, cancel := range s.activeStreams {
			if !validOnes[code] {
				log.Printf("🛡️ SEGURANÇA: Expulsando cliente %s (Licença removida ou desativada no painel)", code)
				cancel()
				delete(s.activeStreams, code)
				delete(s.clients, code)
			}
		}
		s.mu.Unlock()
	}
}

func (s *server) broadcastToWS(data *MachineData) {
	msg, _ := json.Marshal(data)
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if len(s.wsClients) > 0 {
		// log.Printf("📡 Broadcasting to %d WS clients", len(s.wsClients))
	}

	for client := range s.wsClients {
		err := client.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			client.Close()
			delete(s.wsClients, client)
		}
	}
}

func (s *server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	files, err := os.ReadDir(".")
	if err != nil {
		http.Error(w, "Erro ao listar logs", 500)
		return
	}
	var logs []string
	for _, f := range files {
		if !f.IsDir() && strings.HasPrefix(f.Name(), "server-") && strings.HasSuffix(f.Name(), ".log") {
			logs = append(logs, f.Name())
		}
	}
	json.NewEncoder(w).Encode(logs)
}

func (s *server) handleDownloadLog(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("file")
	if filename == "" {
		http.Error(w, "Arquivo não especificado", 400)
		return
	}
	// Segurança básica para evitar path traversal
	if strings.Contains(filename, "..") || !strings.HasPrefix(filename, "server-") {
		http.Error(w, "Acesso negado", 403)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	http.ServeFile(w, r, filename)
}

func (s *server) handleWSLogs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.logClients[conn] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.logClients, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (s *server) startLogBroadcaster() {
	for msg := range logChan {
		s.mu.RLock()
		for client := range s.logClients {
			err := client.WriteMessage(websocket.TextMessage, []byte(msg))
			if err != nil {
				client.Close()
				// We'll clean up disconnected clients later to avoid deadlock or use a separate map
			}
		}
		s.mu.RUnlock()
	}
}

func (s *server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	s.mu.Lock()
	s.wsClients[conn] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.wsClients, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	html := `
<!DOCTYPE html>
<html lang="pt-br">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Auto Monitor Dashboard</title>
    <style>
        :root {
            --bg: #0f172a;
            --card: #1e293b;
            --text: #f8fafc;
            --primary: #3b82f6;
            --success: #10b981;
            --border: #334155;
        }
        body {
            font-family: 'Inter', system-ui, -apple-system, sans-serif;
            background-color: var(--bg);
            color: var(--text);
            margin: 0;
            display: flex;
            flex-direction: column;
            align-items: center;
            min-height: 100vh;
        }
        header {
            width: 100%;
            padding: 2rem 0;
            text-align: center;
            background: linear-gradient(180deg, rgba(59, 130, 246, 0.1) 0%, rgba(15, 23, 42, 0) 100%);
            border-bottom: 1px solid var(--border);
        }
        h1 { margin: 0; font-size: 1.5rem; letter-spacing: -0.025em; }
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
            gap: 1.5rem;
            width: 90%;
            max-width: 1200px;
            padding: 2rem 0;
        }
        .card {
            background-color: var(--card);
            border: 1px solid var(--border);
            border-radius: 12px;
            padding: 1.5rem;
            transition: transform 0.2s;
        }
        .card:hover { transform: translateY(-4px); }
        .machine-id { font-weight: bold; font-size: 1.1rem; margin-bottom: 1rem; color: var(--primary); }
        .metric { margin-bottom: 0.75rem; }
        .label { font-size: 0.75rem; color: #94a3b8; display: flex; justify-content: space-between; }
        .bar-bg { background: #334155; height: 6px; border-radius: 10px; margin-top: 4px; overflow: hidden; }
        .log-container {
            width: 90%;
            max-width: 1200px;
            background: #000;
            border: 1px solid var(--border);
            border-radius: 8px;
            margin-bottom: 2rem;
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }
        .log-header {
            background: var(--border);
            padding: 0.5rem 1rem;
            font-size: 0.70rem;
            color: #94a3b8;
            font-weight: bold;
            display: flex;
            justify-content: space-between;
        }
        #logs {
            height: 200px;
            overflow-y: auto;
            padding: 1rem;
            font-family: 'JetBrains Mono', 'Fira Code', monospace;
            font-size: 0.75rem;
            color: #10b981;
            line-height: 1.4;
            white-space: pre-wrap;
        }
    </style>
</head>
<body>
    <header>
        <h1>🚀 Auto Monitor Live Dashboard</h1>
    </header>
    <div id="machines" class="grid">
        <p style="grid-column: 1/-1; text-align: center; color: #94a3b8;">Aguardando sinal das máquinas...</p>
    </div>

    <div class="log-container">
        <div class="log-header">
            <div>
                <span>SERVER LOGS</span>
                <span id="log-status" style="margin-left: 10px; font-weight: normal; opacity: 0.7;">Conectado</span>
            </div>
            <div>
                <select id="log-list" style="background: #0f172a; color: white; border: 1px solid #334155; font-size: 0.65rem; border-radius: 4px; padding: 2px 5px;">
                    <option>Carregando logs...</option>
                </select>
                <button onclick="downloadLog()" style="background: var(--primary); color: white; border: none; font-size: 0.65rem; padding: 2px 8px; border-radius: 4px; cursor: pointer; margin-left: 5px;">Baixar</button>
            </div>
        </div>
        <div id="logs"></div>
    </div>

    <script>
        const machinesDiv = document.getElementById('machines');
        const logsDiv = document.getElementById('logs');
        const logList = document.getElementById('log-list');

        function fetchLogs() {
            fetch('/list-logs')
                .then(res => res.json())
                .then(data => {
                    logList.innerHTML = data.map(f => "<option value=\"" + f + "\">" + f + "</option>").join("");
                });
        }
        
        function downloadLog() {
            const file = logList.value;
            if (file) window.location.href = "/download-log?file=" + file;
        }

        fetchLogs();
        setInterval(fetchLogs, 30000); // Atualiza lista a cada 30s
        const socket = new WebSocket('ws://' + window.location.host + '/ws');
        const logSocket = new WebSocket('ws://' + window.location.host + '/ws/logs');
        const machines = {};

        socket.onmessage = function(event) {
            const data = JSON.parse(event.data);
            machines[data.code_app] = { ...data, lastSeen: Date.now() };
            updateUI();
        };

        logSocket.onmessage = function(event) {
            const entry = document.createElement('div');
            entry.textContent = event.data;
            logsDiv.appendChild(entry);
            logsDiv.scrollTop = logsDiv.scrollHeight;
            
            // Keep logs limited
            if (logsDiv.childNodes.length > 100) {
                logsDiv.removeChild(logsDiv.firstChild);
            }
        };

        logSocket.onclose = () => document.getElementById('log-status').textContent = 'Desconectado';

        function updateUI() {
            const now = Date.now();
            let html = '';
            
            const activeKeys = Object.keys(machines).filter(k => now - machines[k].lastSeen < 10000);
            
            if (activeKeys.length === 0) {
                machinesDiv.innerHTML = '<p style="grid-column: 1/-1; text-align: center; color: #94a3b8;">Nenhuma máquina ativa no momento.</p>';
                return;
            }

            activeKeys.forEach(id => {
                const m = machines[id];
                html += '<div class="card">' +
                        '<div class="machine-id">' + id + '</div>' +
                        '<div class="metric">' +
                            '<div class="label"><span>CPU</span><span>' + m.cpu_usage.toFixed(1) + '%</span></div>' +
                            '<div class="bar-bg"><div class="bar-fill" style="width: ' + m.cpu_usage + '%; background: var(--primary)"></div></div>' +
                        '</div>' +
                        '<div class="metric">' +
                            '<div class="label"><span>RAM</span><span>' + m.ram_usage.toFixed(1) + '%</span></div>' +
                            '<div class="bar-bg"><div class="bar-fill" style="width: ' + m.ram_usage + '%; background: var(--success)"></div></div>' +
                        '</div>' +
                        '<div class="metric">' +
                            '<div class="label"><span>DISK</span><span>' + m.disk_usage.toFixed(1) + '%</span></div>' +
                            '<div class="bar-bg"><div class="bar-fill" style="width: ' + m.disk_usage + '%; background: #f59e0b"></div></div>' +
                        '</div>' +
                        '<div style="font-size: 0.7rem; color: #64748b; margin-top: 1rem; display: flex; justify-content: space-between;">' +
                            '<span>TX: ' + (m.network_tx/1024).toFixed(1) + ' KB/s</span>' +
                            '<span>RX: ' + (m.network_rx/1024).toFixed(1) + ' KB/s</span>' +
                        '</div>' +
                    '</div>';
            });
            machinesDiv.innerHTML = html;
        }

        setInterval(() => {
            updateUI(); // Clean up old machines
        }, 5000);
    </script>
</body>
</html>
	`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func main() {
	godotenv.Load(".env")

	// Setup Logging to File and Console
	mw := &logWriter{stdout: os.Stdout}
	log.SetOutput(mw)

	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_KEY")

	client, err := supabase.NewClient(supabaseURL, supabaseKey, nil)
	if err != nil {
		log.Fatalf("cannot init supabase: %v", err)
	}

	// InfluxDB Setup
	influxURL := os.Getenv("INFLUX_URL")
	influxToken := os.Getenv("INFLUX_TOKEN")
	influxOrg := os.Getenv("INFLUX_ORG")
	influxBucket := os.Getenv("INFLUX_BUCKET")

	// Configurar InfluxDB com logger silencioso
	influxOptions := influxdb2.DefaultOptions()
	
	influxClient := influxdb2.NewClientWithOptions(influxURL, influxToken, influxOptions)
	writeAPI := influxClient.WriteAPI(influxOrg, influxBucket)

	go func() {
		for err := range writeAPI.Errors() {
			_ = err 
		}
	}()

	s := &server{
		supabaseClient: client,
		influxClient:   influxClient,
		influxWriteAPI: writeAPI,
		clients:        make(map[string]*MachineData),
		wsClients:      make(map[*websocket.Conn]bool),
		logClients:     make(map[*websocket.Conn]bool),
		activeStreams:  make(map[string]context.CancelFunc),
	}
	
	go s.startLogBroadcaster()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		ok, err := influxClient.Ping(ctx)
		if !ok || err != nil {
			s.mu.Lock()
			s.influxWriteAPI = nil
			s.mu.Unlock()
			log.Println("Aviso: InfluxDB offline ou inacessível. O armazenamento histórico está desativado.")
		}
	}()
	
	defer influxClient.Close()

	// gRPC Server
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterMonitorServiceServer(grpcServer, s)

	go s.watchLicenses()

	go func() {
		log.Println("gRPC Server running on :50051")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	// WebSocket/HTTP Server
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": "1.0.4-debug",
			"influx":  os.Getenv("INFLUX_URL"),
		})
	})
	http.HandleFunc("/ws", s.handleWS)
	http.HandleFunc("/monitor/ws", s.handleWS)
	http.HandleFunc("/ws/logs", s.handleWSLogs)
	http.HandleFunc("/monitor/ws/logs", s.handleWSLogs)
	http.HandleFunc("/list-logs", s.handleListLogs)
	http.HandleFunc("/monitor/list-logs", s.handleListLogs)
	http.HandleFunc("/download-log", s.handleDownloadLog)
	http.HandleFunc("/monitor/download-log", s.handleDownloadLog)
	http.HandleFunc("/", s.handleIndex)
	
	log.Println("Web Dashboard running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

