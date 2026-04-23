package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
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
	log.Printf("Auth request for: %s", req.CodeApp)
	
	// Update Supabase flag
	data := map[string]interface{}{"is_connected": true}
	_, _, err := s.supabaseClient.From("licenses_monitor").Update(data, "representation", "minimal").Eq("code_app", req.CodeApp).Execute()
	if err != nil {
		return &pb.AuthResponse{Success: false, Message: err.Error()}, nil
	}

	return &pb.AuthResponse{Success: true, Message: "Connected"}, nil
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
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
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

		s.broadcastToWS(data)
		s.saveToInflux(data)
	}
}

func (s *server) saveToInflux(data *MachineData) {
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

func (s *server) broadcastToWS(data *MachineData) {
	msg, _ := json.Marshal(data)
	s.mu.Lock()
	defer s.mu.Unlock()
	for client := range s.wsClients {
		err := client.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			log.Printf("WS error: %v", err)
			client.Close()
			delete(s.wsClients, client)
		}
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
}

func main() {
	godotenv.Load(".env")

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

	influxClient := influxdb2.NewClient(influxURL, influxToken)
	writeAPI := influxClient.WriteAPI(influxOrg, influxBucket)

	s := &server{
		supabaseClient: client,
		influxClient:   influxClient,
		influxWriteAPI: writeAPI,
		clients:        make(map[string]*MachineData),
		wsClients:      make(map[*websocket.Conn]bool),
	}
	defer influxClient.Close()

	// gRPC Server
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterMonitorServiceServer(grpcServer, s)

	go func() {
		log.Println("gRPC Server running on :50051")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	// WebSocket/HTTP Server
	http.HandleFunc("/ws", s.handleWS)
	log.Println("WebSocket Server running on :8080/ws")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
