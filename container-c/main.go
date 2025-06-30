package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	mmate "github.com/glimte/mmate-go"
	"github.com/glimte/mmate-go/contracts"
	"github.com/glimte/mmate-go/messaging"
)

// Legacy XML format for inventory system
type LegacyInventoryRequest struct {
	XMLName       xml.Name              `xml:"InventoryRequest"`
	TransactionID string                `xml:"transactionId,attr"`
	Action        string                `xml:"action"`
	Items         []LegacyInventoryItem `xml:"items>item"`
}

type LegacyInventoryItem struct {
	SKU      string `xml:"sku"`
	Quantity int    `xml:"quantity"`
}

type LegacyInventoryResponse struct {
	XMLName       xml.Name                    `xml:"InventoryResponse"`
	TransactionID string                      `xml:"transactionId,attr"`
	Status        string                      `xml:"status"`
	Items         []LegacyInventoryItemStatus `xml:"items>item"`
}

type LegacyInventoryItemStatus struct {
	SKU           string `xml:"sku"`
	Available     bool   `xml:"available"`
	ReservationID string `xml:"reservationId,omitempty"`
}

// Legacy JSON format for shipping system
type LegacyShippingRequest struct {
	RefNum        string               `json:"ref_num"`
	DestAddr      LegacyAddress        `json:"dest_addr"`
	ShipmentItems []LegacyShipmentItem `json:"shipment_items"`
	Priority      string               `json:"priority"`
	Instructions  string               `json:"special_instructions,omitempty"`
}

type LegacyAddress struct {
	AddrLine1 string `json:"addr_line_1"`
	AddrLine2 string `json:"addr_line_2,omitempty"`
	CityName  string `json:"city_name"`
	StateCode string `json:"state_code"`
	PostCode  string `json:"post_code"`
	Country   string `json:"country_code"`
}

type LegacyShipmentItem struct {
	ItemCode string  `json:"item_code"`
	ItemQty  int     `json:"item_qty"`
	Weight   float64 `json:"weight_lbs"`
}

type LegacyShippingResponse struct {
	Status      string    `json:"status"`
	TrackingNum string    `json:"tracking_num"`
	Carrier     string    `json:"carrier"`
	EstDelivery string    `json:"est_delivery"`
	Cost        float64   `json:"cost_usd"`
	ServiceType string    `json:"service_type"`
	CreatedAt   time.Time `json:"created_at"`
}

// Modern message formats
type CheckInventoryCommand struct {
	contracts.BaseCommand
	OrderID string      `json:"orderId"`
	Items   []OrderItem `json:"items"`
}

type InventoryCheckedReply struct {
	contracts.BaseReply
	OrderID         string                 `json:"orderId"`
	Available       bool                   `json:"available"`
	ReservationIDs  map[string]string      `json:"reservationIds"`
	UnavailableItems []string              `json:"unavailableItems,omitempty"`
}

type ArrangeShippingCommand struct {
	contracts.BaseCommand
	OrderID         string          `json:"orderId"`
	Items           []OrderItem     `json:"items"`
	ShippingAddress ShippingAddress `json:"shippingAddress"`
	Priority        string          `json:"priority"`
}

type ShippingArrangedReply struct {
	contracts.BaseReply
	OrderID           string    `json:"orderId"`
	TrackingNumber    string    `json:"trackingNumber"`
	Carrier           string    `json:"carrier"`
	EstimatedDelivery time.Time `json:"estimatedDelivery"`
	ShippingCost      float64   `json:"shippingCost"`
}

type OrderItem struct {
	ProductID   string  `json:"productId"`
	ProductName string  `json:"productName"`
	Quantity    int     `json:"quantity"`
	Price       float64 `json:"price"`
}

type ShippingAddress struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zipCode"`
	Country string `json:"country"`
}

// Integration metrics
type IntegrationMetrics struct {
	InventoryChecks     int
	ShipmentsArranged   int
	XMLTransformations  int
	JSONTransformations int
	LegacyCallDuration  time.Duration
}

var metrics = &IntegrationMetrics{}

func main() {
	log.Println("🚀 [Container C] Integration Service")
	log.Println("🔄 Message Transformation & Legacy System Integration")
	log.Println("📊 Demonstrating: JSON ↔ XML transformations, Legacy API adapters")

	// Register message types
	messaging.Register("CheckInventoryCommand", func() contracts.Message { return &CheckInventoryCommand{} })
	messaging.Register("InventoryCheckedReply", func() contracts.Message { return &InventoryCheckedReply{} })
	messaging.Register("ArrangeShippingCommand", func() contracts.Message { return &ArrangeShippingCommand{} })
	messaging.Register("ShippingArrangedReply", func() contracts.Message { return &ShippingArrangedReply{} })

	// Wait for RabbitMQ
	time.Sleep(10 * time.Second)

	ctx := context.Background()

	// Create mmate client with queue bindings
	client, err := mmate.NewClientWithOptions("amqp://admin:admin@rabbitmq:5672/",
		mmate.WithServiceName("integration-service"),
		mmate.WithQueueBindings(
			messaging.QueueBinding{
				Exchange:   "mmate.commands",
				RoutingKey: "cmd.integration-service.CheckInventoryCommand",
			},
			messaging.QueueBinding{
				Exchange:   "mmate.commands", 
				RoutingKey: "cmd.integration-service.ArrangeShippingCommand",
			},
		),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Get components
	publisher := client.Publisher()
	subscriber := client.Subscriber()
	dispatcher := client.Dispatcher()

	log.Println("✅ Connected to RabbitMQ")

	// Handler for inventory check commands
	inventoryHandler := messaging.MessageHandlerFunc(func(ctx context.Context, msg contracts.Message) error {
		cmd, ok := msg.(*CheckInventoryCommand)
		if !ok {
			return nil
		}

		log.Printf("\n📦 Processing Inventory Check")
		log.Printf("   Order ID: %s", cmd.OrderID)
		log.Printf("   Items: %d", len(cmd.Items))

		metrics.InventoryChecks++
		metrics.XMLTransformations++

		// Transform to legacy XML format
		legacyReq := LegacyInventoryRequest{
			TransactionID: fmt.Sprintf("TXN-%s-%d", cmd.OrderID, time.Now().Unix()),
			Action:        "RESERVE",
			Items:         []LegacyInventoryItem{},
		}

		for _, item := range cmd.Items {
			legacyReq.Items = append(legacyReq.Items, LegacyInventoryItem{
				SKU:      item.ProductID,
				Quantity: item.Quantity,
			})
		}

		// Marshal to XML
		xmlData, _ := xml.MarshalIndent(legacyReq, "", "  ")
		log.Printf("🔄 Transformed to XML (%d bytes):\n%s", len(xmlData), string(xmlData))

		// Simulate legacy system call
		startTime := time.Now()
		time.Sleep(200 * time.Millisecond + time.Duration(rand.Intn(100))*time.Millisecond)
		
		// Simulate legacy response
		available := rand.Float64() > 0.1 // 90% success rate
		legacyResp := LegacyInventoryResponse{
			TransactionID: legacyReq.TransactionID,
			Status:        "PROCESSED",
			Items:         []LegacyInventoryItemStatus{},
		}

		reservationIDs := make(map[string]string)
		unavailableItems := []string{}

		for _, item := range legacyReq.Items {
			itemAvailable := available
			if itemAvailable {
				resID := fmt.Sprintf("RES-%s-%d", item.SKU, time.Now().UnixNano())
				reservationIDs[item.SKU] = resID
				legacyResp.Items = append(legacyResp.Items, LegacyInventoryItemStatus{
					SKU:           item.SKU,
					Available:     true,
					ReservationID: resID,
				})
			} else {
				unavailableItems = append(unavailableItems, item.SKU)
				legacyResp.Items = append(legacyResp.Items, LegacyInventoryItemStatus{
					SKU:       item.SKU,
					Available: false,
				})
			}
		}

		metrics.LegacyCallDuration += time.Since(startTime)

		// Transform response back
		respXML, _ := xml.MarshalIndent(legacyResp, "", "  ")
		log.Printf("📥 Legacy response:\n%s", string(respXML))

		// Create reply
		reply := &InventoryCheckedReply{}
		reply.Type = "InventoryCheckedReply"
		reply.ID = fmt.Sprintf("reply-%s", cmd.ID)
		reply.Timestamp = time.Now()
		reply.CorrelationID = cmd.GetCorrelationID()
		reply.OrderID = cmd.OrderID
		reply.Available = available
		reply.ReservationIDs = reservationIDs
		reply.UnavailableItems = unavailableItems
		reply.Success = true

		if !available {
			log.Printf("❌ Inventory unavailable for some items")
		} else {
			log.Printf("✅ Inventory reserved successfully")
		}

		// Send reply
		log.Printf("📤 ReplyTo field: '%s'", cmd.ReplyTo)
		log.Printf("📤 Reply CorrelationID: '%s'", reply.CorrelationID)
		if cmd.ReplyTo != "" {
			// Use PublishReply which correctly sets empty exchange
			err := publisher.PublishReply(ctx, reply, cmd.ReplyTo)
			if err != nil {
				log.Printf("❌ Failed to send reply: %v", err)
				return err
			}
			log.Printf("📤 Reply sent to: %s", cmd.ReplyTo)
		} else {
			log.Printf("⚠️  No ReplyTo address - cannot send reply")
		}

		return nil
	})

	// Handler for shipping arrangement commands
	shippingHandler := messaging.MessageHandlerFunc(func(ctx context.Context, msg contracts.Message) error {
		cmd, ok := msg.(*ArrangeShippingCommand)
		if !ok {
			return nil
		}

		log.Printf("\n🚚 Processing Shipping Arrangement")
		log.Printf("   Order ID: %s", cmd.OrderID)
		log.Printf("   Destination: %s, %s", cmd.ShippingAddress.City, cmd.ShippingAddress.State)

		metrics.ShipmentsArranged++
		metrics.JSONTransformations++

		// Transform to legacy JSON format
		legacyReq := LegacyShippingRequest{
			RefNum:   cmd.OrderID,
			Priority: cmd.Priority,
			DestAddr: LegacyAddress{
				AddrLine1: cmd.ShippingAddress.Street,
				CityName:  cmd.ShippingAddress.City,
				StateCode: cmd.ShippingAddress.State,
				PostCode:  cmd.ShippingAddress.ZipCode,
				Country:   "US",
			},
			ShipmentItems: []LegacyShipmentItem{},
		}

		totalWeight := 0.0
		for _, item := range cmd.Items {
			// Simulate weight calculation
			weight := float64(item.Quantity) * 1.5
			totalWeight += weight
			legacyReq.ShipmentItems = append(legacyReq.ShipmentItems, LegacyShipmentItem{
				ItemCode: item.ProductID,
				ItemQty:  item.Quantity,
				Weight:   weight,
			})
		}

		// Marshal to JSON
		jsonData, _ := json.MarshalIndent(legacyReq, "", "  ")
		log.Printf("🔄 Transformed to JSON (%d bytes):\n%s", len(jsonData), string(jsonData))

		// Simulate legacy system call
		startTime := time.Now()
		time.Sleep(300 * time.Millisecond + time.Duration(rand.Intn(200))*time.Millisecond)

		// Simulate legacy response
		carrier := "FedEx"
		if totalWeight > 50 {
			carrier = "Freight"
		}
		
		trackingNum := fmt.Sprintf("1Z%09d", rand.Intn(999999999))
		deliveryDays := 3
		if cmd.Priority == "EXPRESS" {
			deliveryDays = 1
		}
		
		cost := 15.99 + (totalWeight * 0.75)
		if cmd.Priority == "EXPRESS" {
			cost *= 2.5
		}

		legacyResp := LegacyShippingResponse{
			Status:      "CONFIRMED",
			TrackingNum: trackingNum,
			Carrier:     carrier,
			EstDelivery: time.Now().Add(time.Duration(deliveryDays) * 24 * time.Hour).Format("2006-01-02"),
			Cost:        cost,
			ServiceType: cmd.Priority,
			CreatedAt:   time.Now(),
		}

		metrics.LegacyCallDuration += time.Since(startTime)

		// Log response
		respJSON, _ := json.MarshalIndent(legacyResp, "", "  ")
		log.Printf("📥 Legacy response:\n%s", string(respJSON))

		// Create reply
		estDelivery, _ := time.Parse("2006-01-02", legacyResp.EstDelivery)
		reply := &ShippingArrangedReply{}
		reply.Type = "ShippingArrangedReply"
		reply.ID = fmt.Sprintf("reply-%s", cmd.ID)
		reply.Timestamp = time.Now()
		reply.CorrelationID = cmd.GetCorrelationID()
		reply.OrderID = cmd.OrderID
		reply.TrackingNumber = legacyResp.TrackingNum
		reply.Carrier = legacyResp.Carrier
		reply.EstimatedDelivery = estDelivery
		reply.ShippingCost = legacyResp.Cost
		reply.Success = true

		log.Printf("✅ Shipping arranged successfully")
		log.Printf("   Tracking: %s", trackingNum)
		log.Printf("   Cost: $%.2f", cost)

		// Send reply
		log.Printf("📤 ReplyTo field: '%s'", cmd.ReplyTo)
		log.Printf("📤 Reply CorrelationID: '%s'", reply.CorrelationID)
		if cmd.ReplyTo != "" {
			// Use PublishReply which correctly sets empty exchange
			err := publisher.PublishReply(ctx, reply, cmd.ReplyTo)
			if err != nil {
				log.Printf("❌ Failed to send reply: %v", err)
				return err
			}
			log.Printf("📤 Reply sent to: %s", cmd.ReplyTo)
		} else {
			log.Printf("⚠️  No ReplyTo address - cannot send reply")
		}

		return nil
	})

	// Register handlers
	err = dispatcher.RegisterHandler(&CheckInventoryCommand{}, inventoryHandler)
	if err != nil {
		log.Fatalf("Failed to register inventory handler: %v", err)
	}

	err = dispatcher.RegisterHandler(&ArrangeShippingCommand{}, shippingHandler)
	if err != nil {
		log.Fatalf("Failed to register shipping handler: %v", err)
	}

	// Subscribe to the queue once with the dispatcher handling all message types
	err = subscriber.Subscribe(ctx, client.ServiceQueue(), "integration-messages", dispatcher,
		messaging.WithAutoAck(true),
	)
	if err != nil {
		log.Fatalf("Failed to subscribe to integration queue: %v", err)
	}

	log.Printf("✅ Subscribed to queue: %s", client.ServiceQueue())
	log.Println("\n🎯 Integration Service ready")
	log.Println("📊 Patterns demonstrated:")
	log.Println("   • Message Transformation (JSON ↔ XML)")
	log.Println("   • Legacy System Integration")
	log.Println("   • Protocol Bridging")
	log.Println("   • Data Mapping & Enrichment")

	// Periodic metrics reporting
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			if metrics.InventoryChecks > 0 || metrics.ShipmentsArranged > 0 {
				avgLatency := time.Duration(0)
				totalOps := metrics.InventoryChecks + metrics.ShipmentsArranged
				if totalOps > 0 {
					avgLatency = metrics.LegacyCallDuration / time.Duration(totalOps)
				}
				
				log.Printf("\n📊 Integration Metrics:")
				log.Printf("   Inventory checks: %d", metrics.InventoryChecks)
				log.Printf("   Shipments arranged: %d", metrics.ShipmentsArranged)
				log.Printf("   XML transformations: %d", metrics.XMLTransformations)
				log.Printf("   JSON transformations: %d", metrics.JSONTransformations)
				log.Printf("   Avg legacy call latency: %v", avgLatency)
			}
		}
	}()

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	ticker.Stop()
	log.Println("\n✅ Integration service shutting down")
}