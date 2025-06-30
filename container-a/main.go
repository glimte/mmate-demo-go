package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	mmate "github.com/glimte/mmate-go"
	"github.com/glimte/mmate-go/bridge"
	"github.com/glimte/mmate-go/contracts"
	"github.com/glimte/mmate-go/messaging"
)

// Order request from HTTP
type OrderRequest struct {
	CustomerID   string      `json:"customerId" binding:"required"`
	CustomerName string      `json:"customerName" binding:"required"`
	Email        string      `json:"email" binding:"required,email"`
	Items        []OrderItem `json:"items" binding:"required,min=1"`
}

type OrderItem struct {
	ProductID   string  `json:"productId" binding:"required"`
	ProductName string  `json:"productName" binding:"required"`
	Quantity    int     `json:"quantity" binding:"required,min=1"`
	Price       float64 `json:"price" binding:"required,min=0"`
}

// Command to send to order processor
type ProcessOrderCommand struct {
	contracts.BaseCommand
	OrderID      string      `json:"orderId"`
	CustomerID   string      `json:"customerId"`
	CustomerName string      `json:"customerName"`
	Email        string      `json:"email"`
	Items        []OrderItem `json:"items"`
	TotalAmount  float64     `json:"totalAmount"`
}

// Reply from order processor
type OrderProcessedReply struct {
	contracts.BaseReply
	OrderID    string `json:"orderId"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	ProcessedAt time.Time `json:"processedAt"`
}

func main() {
	log.Println("🚀 [Container A] API Gateway with SyncAsync Bridge")
	log.Println("🌐 Converting HTTP requests to async messages with sync responses")

	// Register message types
	messaging.Register("ProcessOrderCommand", func() contracts.Message { return &ProcessOrderCommand{} })
	messaging.Register("OrderProcessedReply", func() contracts.Message { return &OrderProcessedReply{} })

	// Wait for RabbitMQ
	time.Sleep(10 * time.Second)

	// Create mmate client
	client, err := mmate.NewClientWithOptions("amqp://admin:admin@rabbitmq:5672/",
		mmate.WithServiceName("api-gateway"),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Get publisher and subscriber
	publisher := client.Publisher()
	subscriber := client.Subscriber()

	log.Println("✅ Connected to RabbitMQ")

	// Create SyncAsync Bridge
	syncAsyncBridge, err := bridge.NewSyncAsyncBridge(publisher, subscriber, nil)
	if err != nil {
		log.Fatalf("Failed to create sync-async bridge: %v", err)
	}
	defer syncAsyncBridge.Close()

	log.Println("✅ SyncAsync Bridge initialized")

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/health"},
	}))

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"service": "api-gateway",
			"bridge":  "active",
		})
	})

	// Create order endpoint - uses Bridge for sync request/response
	router.POST("/api/orders", func(c *gin.Context) {
		var req OrderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Calculate total
		var total float64
		for _, item := range req.Items {
			total += item.Price * float64(item.Quantity)
		}

		// Create command
		cmd := &ProcessOrderCommand{}
		cmd.Type = "ProcessOrderCommand"
		cmd.ID = uuid.New().String()
		cmd.Timestamp = time.Now()
		cmd.OrderID = fmt.Sprintf("ORD-%s", uuid.New().String()[:8])
		cmd.CustomerID = req.CustomerID
		cmd.CustomerName = req.CustomerName
		cmd.Email = req.Email
		cmd.Items = req.Items
		cmd.TotalAmount = total
		cmd.TargetService = "order-processor" // Route to order processor

		log.Printf("📤 Sending order command via Bridge: %s", cmd.OrderID)
		log.Printf("   Customer: %s", cmd.CustomerName)
		log.Printf("   Total: $%.2f", cmd.TotalAmount)

		// Use Bridge to send command and wait for response (type-safe)
		orderReply, err := bridge.RequestCommandTyped[*OrderProcessedReply](syncAsyncBridge, c.Request.Context(), cmd, 30*time.Second)
		
		if err != nil {
			log.Printf("❌ Bridge request failed: %v", err)
			c.JSON(http.StatusGatewayTimeout, gin.H{
				"error": "Order processing timeout",
				"orderId": cmd.OrderID,
			})
			return
		}

		log.Printf("✅ Received reply for order: %s", orderReply.OrderID)
		log.Printf("   Status: %s", orderReply.Status)
		log.Printf("   Message: %s", orderReply.Message)

		// Return response to HTTP client
		if orderReply.Success {
			c.JSON(http.StatusOK, gin.H{
				"orderId": orderReply.OrderID,
				"status": orderReply.Status,
				"message": orderReply.Message,
				"processedAt": orderReply.ProcessedAt,
			})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": orderReply.Message,
				"orderId": orderReply.OrderID,
			})
		}
	})

	// Demo endpoint
	router.GET("/api/demo", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Post an order to /api/orders",
			"example": gin.H{
				"customerId": "CUST-123",
				"customerName": "John Doe",
				"email": "john@example.com",
				"items": []gin.H{
					{
						"productId": "PROD-001",
						"productName": "Laptop",
						"quantity": 1,
						"price": 999.99,
					},
				},
			},
		})
	})

	log.Println("✅ API Gateway ready")
	log.Println("🌐 Listening on :8080")
	log.Println("📡 Using SyncAsync Bridge for request/response")

	// Start HTTP server
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}