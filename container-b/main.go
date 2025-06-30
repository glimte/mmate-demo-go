package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	mmate "github.com/glimte/mmate-go"
	"github.com/glimte/mmate-go/bridge"
	"github.com/glimte/mmate-go/contracts"
	"github.com/glimte/mmate-go/messaging"
	"github.com/glimte/mmate-go/stageflow"
)

// Order item
type OrderItem struct {
	ProductID   string  `json:"productId"`
	ProductName string  `json:"productName"`
	Quantity    int     `json:"quantity"`
	Price       float64 `json:"price"`
}

// Shipping address
type ShippingAddress struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zipCode"`
	Country string `json:"country"`
}

// Command from API Gateway
type ProcessOrderCommand struct {
	contracts.BaseCommand
	OrderID         string          `json:"orderId"`
	CustomerID      string          `json:"customerId"`
	CustomerName    string          `json:"customerName"`
	Email           string          `json:"email"`
	Items           []OrderItem     `json:"items"`
	TotalAmount     float64         `json:"totalAmount"`
	ShippingAddress ShippingAddress `json:"shippingAddress"`
}

// Reply to API Gateway
type OrderProcessedReply struct {
	contracts.BaseReply
	OrderID     string    `json:"orderId"`
	Status      string    `json:"status"`
	Message     string    `json:"message"`
	ProcessedAt time.Time `json:"processedAt"`
}

// Commands for Container C
type CheckInventoryCommand struct {
	contracts.BaseCommand
	OrderID string      `json:"orderId"`
	Items   []OrderItem `json:"items"`
}

type InventoryCheckedReply struct {
	contracts.BaseReply
	OrderID          string            `json:"orderId"`
	Available        bool              `json:"available"`
	ReservationIDs   map[string]string `json:"reservationIds"`
	UnavailableItems []string          `json:"unavailableItems,omitempty"`
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

// OrderWorkflowContext is the typed context for the order workflow
type OrderWorkflowContext struct {
	// Input data
	OrderID         string          `json:"orderId"`
	CustomerID      string          `json:"customerId"`
	CustomerName    string          `json:"customerName"`
	Email           string          `json:"email"`
	Items           []OrderItem     `json:"items"`
	TotalAmount     float64         `json:"totalAmount"`
	ShippingAddress ShippingAddress `json:"shippingAddress"`
	ReceivedAt      time.Time       `json:"receivedAt"`
	
	// Stage results
	Validated         bool              `json:"validated,omitempty"`
	ValidatedAt       *time.Time        `json:"validatedAt,omitempty"`
	InventoryReserved bool              `json:"inventoryReserved,omitempty"`
	ReservationIDs    map[string]string `json:"reservationIds,omitempty"`
	CheckedAt         *time.Time        `json:"checkedAt,omitempty"`
	PaymentID         string            `json:"paymentId,omitempty"`
	PaymentAmount     float64           `json:"paymentAmount,omitempty"`
	PaymentMethod     string            `json:"paymentMethod,omitempty"`
	ProcessedAt       *time.Time        `json:"processedAt,omitempty"`
	ShippingArranged  bool              `json:"shippingArranged,omitempty"`
	TrackingNumber    string            `json:"trackingNumber,omitempty"`
	Carrier           string            `json:"carrier,omitempty"`
	EstimatedDelivery *time.Time        `json:"estimatedDelivery,omitempty"`
	ShippingCost      float64           `json:"shippingCost,omitempty"`
	ArrangedAt        *time.Time        `json:"arrangedAt,omitempty"`
}

// Validate implements TypedWorkflowContext
func (c *OrderWorkflowContext) Validate() error {
	if c.CustomerID == "" {
		return fmt.Errorf("customer ID is required")
	}
	if c.OrderID == "" {
		return fmt.Errorf("order ID is required")
	}
	if len(c.Items) == 0 {
		return fmt.Errorf("at least one item is required")
	}
	return nil
}

// Stage handlers - now using typed interface
type ValidateOrderStage struct{}

func (s *ValidateOrderStage) Execute(ctx context.Context, order *OrderWorkflowContext) error {
	log.Printf("📋 [Stage: Validate] Processing order: %s", order.OrderID)
	
	// Simulate validation
	time.Sleep(200 * time.Millisecond)
	
	// Direct typed access - no type assertions!
	if order.CustomerID == "" {
		return fmt.Errorf("validation failed: customer ID required")
	}
	
	if order.Email == "" {
		return fmt.Errorf("validation failed: email required")
	}
	
	if len(order.Items) == 0 {
		return fmt.Errorf("validation failed: at least one item required")
	}
	
	// Validate items
	for i, item := range order.Items {
		if item.Quantity <= 0 {
			return fmt.Errorf("validation failed: invalid quantity for item %d", i)
		}
		if item.Price < 0 {
			return fmt.Errorf("validation failed: invalid price for item %d", i)
		}
	}
	
	log.Printf("✅ Order validation passed")
	
	// Update the context with validation result
	order.Validated = true
	now := time.Now()
	order.ValidatedAt = &now
	
	return nil
}

func (s *ValidateOrderStage) GetStageID() string {
	return "validate-order"
}

type CheckInventoryStage struct {
	Bridge *bridge.SyncAsyncBridge
}

func (s *CheckInventoryStage) Execute(ctx context.Context, order *OrderWorkflowContext) error {
	log.Printf("📦 [Stage: Inventory] Processing order: %s", order.OrderID)
	
	// Check if order is validated
	if !order.Validated {
		return fmt.Errorf("cannot check inventory: order not validated")
	}
	
	// Create inventory check command - no type assertions needed!
	cmd := &CheckInventoryCommand{
		BaseCommand: contracts.BaseCommand{
			BaseMessage: contracts.BaseMessage{
				Type:      "CheckInventoryCommand",
				ID:        fmt.Sprintf("inv-check-%d", time.Now().UnixNano()),
				Timestamp: time.Now(),
			},
			TargetService: "integration-service",
		},
		OrderID: order.OrderID,
		Items:   order.Items, // Direct access to typed items!
	}
	
	log.Printf("🔄 Sending inventory check to integration service")
	
	// Use Bridge to call Container C (type-safe)
	invReply, err := bridge.RequestCommandTyped[*InventoryCheckedReply](s.Bridge, ctx, cmd, 10*time.Second)
	if err != nil {
		return fmt.Errorf("inventory check failed: %w", err)
	}
	
	if !invReply.Available {
		log.Printf("❌ Inventory unavailable: %v", invReply.UnavailableItems)
		return fmt.Errorf("inventory unavailable for items: %v", invReply.UnavailableItems)
	}
	
	log.Printf("✅ Inventory available, reservations: %v", invReply.ReservationIDs)
	
	// Update the context with inventory results
	order.InventoryReserved = true
	order.ReservationIDs = invReply.ReservationIDs
	now := time.Now()
	order.CheckedAt = &now
	
	return nil
}

func (s *CheckInventoryStage) GetStageID() string {
	return "check-inventory"
}

type ProcessPaymentStage struct{}

func (s *ProcessPaymentStage) Execute(ctx context.Context, order *OrderWorkflowContext) error {
	log.Printf("💳 [Stage: Payment] Processing order: %s", order.OrderID)
	
	// Check prerequisites
	if !order.InventoryReserved {
		return fmt.Errorf("cannot process payment: inventory not reserved")
	}
	
	// Long delay to allow container restart during processing
	log.Printf("💳 [Payment] Simulating long payment processing (5 seconds)...")
	time.Sleep(5 * time.Second)
	
	// Reject very large amounts
	if order.TotalAmount > 10000 {
		return fmt.Errorf("payment failed: amount too large ($%.2f)", order.TotalAmount)
	}
	
	log.Printf("✅ Payment processed: $%.2f", order.TotalAmount)
	
	// Update the context with payment results
	order.PaymentID = fmt.Sprintf("PAY-%d", time.Now().Unix())
	order.PaymentAmount = order.TotalAmount
	order.PaymentMethod = "credit_card"
	now := time.Now()
	order.ProcessedAt = &now
	
	return nil
}

func (s *ProcessPaymentStage) GetStageID() string {
	return "process-payment"
}

type ArrangeShippingStage struct {
	Bridge *bridge.SyncAsyncBridge
}

func (s *ArrangeShippingStage) Execute(ctx context.Context, order *OrderWorkflowContext) error {
	log.Printf("🚚 [Stage: Shipping] Processing order: %s", order.OrderID)
	
	// Check prerequisites
	if order.PaymentID == "" {
		return fmt.Errorf("cannot arrange shipping: payment not processed")
	}
	
	// Create shipping command - no type assertions needed!
	cmd := &ArrangeShippingCommand{
		BaseCommand: contracts.BaseCommand{
			BaseMessage: contracts.BaseMessage{
				Type:      "ArrangeShippingCommand",
				ID:        fmt.Sprintf("ship-arrange-%d", time.Now().UnixNano()),
				Timestamp: time.Now(),
			},
			TargetService: "integration-service",
		},
		OrderID:         order.OrderID,
		Items:           order.Items, // Direct access!
		ShippingAddress: order.ShippingAddress, // Direct access!
		Priority:        "STANDARD",
	}
	
	log.Printf("🔄 Sending shipping arrangement to integration service")
	
	// Use Bridge to call Container C (type-safe)
	shipReply, err := bridge.RequestCommandTyped[*ShippingArrangedReply](s.Bridge, ctx, cmd, 10*time.Second)
	if err != nil {
		return fmt.Errorf("shipping arrangement failed: %w", err)
	}
	
	log.Printf("✅ Shipping arranged: %s (via %s)", shipReply.TrackingNumber, shipReply.Carrier)
	log.Printf("   Estimated delivery: %s", shipReply.EstimatedDelivery.Format("2006-01-02"))
	log.Printf("   Cost: $%.2f", shipReply.ShippingCost)
	
	// Update the context with shipping results
	order.ShippingArranged = true
	order.TrackingNumber = shipReply.TrackingNumber
	order.Carrier = shipReply.Carrier
	delivery := shipReply.EstimatedDelivery
	order.EstimatedDelivery = &delivery
	order.ShippingCost = shipReply.ShippingCost
	now := time.Now()
	order.ArrangedAt = &now
	
	return nil
}

func (s *ArrangeShippingStage) GetStageID() string {
	return "arrange-shipping"
}

func main() {
	log.Println("🚀 [Container B] Order Processor with StageFlow")
	log.Println("📊 Multi-stage order processing with 100% delivery guarantee")
	log.Println("🌉 Using Bridge to integrate with Container C for inventory & shipping")

	// Register message types
	messaging.Register("ProcessOrderCommand", func() contracts.Message { return &ProcessOrderCommand{} })
	messaging.Register("OrderProcessedReply", func() contracts.Message { return &OrderProcessedReply{} })
	messaging.Register("CheckInventoryCommand", func() contracts.Message { return &CheckInventoryCommand{} })
	messaging.Register("InventoryCheckedReply", func() contracts.Message { return &InventoryCheckedReply{} })
	messaging.Register("ArrangeShippingCommand", func() contracts.Message { return &ArrangeShippingCommand{} })
	messaging.Register("ShippingArrangedReply", func() contracts.Message { return &ShippingArrangedReply{} })
	// Register StageFlow message type (registered by engine but ensure it's available)
	messaging.Register("FlowMessageEnvelope", func() contracts.Message { return &stageflow.FlowMessageEnvelope{} })

	// Wait for RabbitMQ
	time.Sleep(10 * time.Second)

	ctx := context.Background()

	// Create mmate client with queue bindings
	client, err := mmate.NewClientWithOptions("amqp://admin:admin@rabbitmq:5672/",
		mmate.WithServiceName("order-processor"),
		mmate.WithQueueBindings(
			messaging.QueueBinding{
				Exchange:   "mmate.commands",
				RoutingKey: "cmd.order-processor.ProcessOrderCommand",
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

	// Get Bridge from client - it handles queue creation properly
	integrationBridge := client.Bridge()
	if integrationBridge == nil {
		log.Fatal("Failed to create integration bridge")
	}
	defer integrationBridge.Close()

	log.Println("✅ Integration Bridge initialized")

	// Create StageFlow engine
	stageFlowEngine := stageflow.NewStageFlowEngine(publisher, subscriber)
	
	// Set the service queue for workflow message routing
	stageFlowEngine.SetServiceQueue(client.ServiceQueue())

	// Define order processing workflow using typed wrapper
	orderWorkflow := stageflow.NewTypedWorkflow[*OrderWorkflowContext]("order-processing", "Order Processing Workflow").
		AddTypedStage("validate-order", &ValidateOrderStage{}).
		AddTypedStage("check-inventory", &CheckInventoryStage{Bridge: integrationBridge}).
		AddTypedStage("process-payment", &ProcessPaymentStage{}).
		AddTypedStage("arrange-shipping", &ArrangeShippingStage{Bridge: integrationBridge}).
		Build()

	// Register workflow
	err = stageFlowEngine.RegisterWorkflow(orderWorkflow)
	if err != nil {
		log.Fatalf("Failed to register workflow: %v", err)
	}

	log.Println("✅ StageFlow workflow registered")
	log.Println("📋 Stages: Validate → Inventory (via C) → Payment → Shipping (via C)")

	// Handler for incoming commands
	commandHandler := messaging.MessageHandlerFunc(func(ctx context.Context, msg contracts.Message) error {
		cmd, ok := msg.(*ProcessOrderCommand)
		if !ok {
			return nil
		}

		log.Printf("\n📨 Received order command: %s", cmd.OrderID)
		log.Printf("   Customer: %s", cmd.CustomerName)
		log.Printf("   Items: %d", len(cmd.Items))
		log.Printf("   Total: $%.2f", cmd.TotalAmount)

		// Prepare typed workflow context
		orderContext := &OrderWorkflowContext{
			OrderID:         cmd.OrderID,
			CustomerID:      cmd.CustomerID,
			CustomerName:    cmd.CustomerName,
			Email:           cmd.Email,
			Items:           cmd.Items,
			TotalAmount:     cmd.TotalAmount,
			ShippingAddress: cmd.ShippingAddress,
			ReceivedAt:      time.Now(),
		}

		// Execute workflow using typed StageFlow (queue-based - asynchronous)
		log.Printf("🔄 Starting StageFlow workflow for order: %s", cmd.OrderID)
		state, err := stageflow.ExecuteTyped(orderWorkflow, ctx, orderContext)

		// Prepare reply - in queue-based mode, we reply immediately with "started" status
		reply := &OrderProcessedReply{
			BaseReply: contracts.BaseReply{
				BaseMessage: contracts.BaseMessage{
					Type:          "OrderProcessedReply",
					ID:            fmt.Sprintf("reply-%s", cmd.ID),
					Timestamp:     time.Now(),
					CorrelationID: cmd.GetCorrelationID(),
				},
			},
			OrderID:     cmd.OrderID,
			ProcessedAt: time.Now(),
		}

		if err != nil {
			log.Printf("❌ Failed to start workflow: %v", err)
			reply.BaseReply.Success = false
			reply.Status = "failed"
			reply.Message = fmt.Sprintf("Failed to start order processing: %v", err)
		} else {
			log.Printf("✅ Workflow started successfully - processing asynchronously")
			reply.BaseReply.Success = true
			reply.Status = "processing" // Changed from "completed" to "processing"
			reply.Message = fmt.Sprintf("Order processing started - workflow ID: %s", state.InstanceID)
		}

		// Send reply back to API Gateway
		if cmd.ReplyTo != "" {
			log.Printf("📤 Sending reply to: %s", cmd.ReplyTo)
			// Use PublishReply which correctly sets empty exchange
			err = publisher.PublishReply(ctx, reply, cmd.ReplyTo)
			if err != nil {
				log.Printf("❌ Failed to send reply: %v", err)
			}
		}

		log.Printf("📊 Workflow state: %s (processing asynchronously)", state.Status)
		log.Printf("   Instance ID: %s", state.InstanceID)
		log.Printf("   Stages will execute via queue messages")
		
		return nil
	})

	// Register command handler
	err = dispatcher.RegisterHandler(&ProcessOrderCommand{}, commandHandler)
	if err != nil {
		log.Fatalf("Failed to register command handler: %v", err)
	}

	// Register StageFlow handler for workflow messages
	stageFlowHandler := messaging.MessageHandlerFunc(func(ctx context.Context, msg contracts.Message) error {
		envelope, ok := msg.(*stageflow.FlowMessageEnvelope)
		if !ok {
			return nil // Not a workflow message, ignore
		}
		
		log.Printf("🔄 Received stage message: workflow=%s, instance=%s, stage=%s",
			envelope.WorkflowID, envelope.InstanceID, envelope.StageName)
		
		// Find the workflow and process the message
		workflow, err := stageFlowEngine.GetWorkflow(envelope.WorkflowID)
		if err != nil {
			log.Printf("❌ Workflow not found: %v", err)
			return err
		}
		
		// Process the stage message directly
		return workflow.ProcessStageMessage(ctx, envelope)
	})
	
	err = dispatcher.RegisterHandler(&stageflow.FlowMessageEnvelope{}, stageFlowHandler)
	if err != nil {
		log.Fatalf("Failed to register stageflow handler: %v", err)
	}

	// Subscribe to service queue for multiple message types
	// Use manual acknowledgment for true state persistence and crash recovery
	err = subscriber.Subscribe(ctx, client.ServiceQueue(), "*", dispatcher,
		messaging.WithAutoAck(false),
	)
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	log.Printf("✅ Subscribed to queue: %s", client.ServiceQueue())
	log.Println("\n🎯 Order Processor ready with StageFlow")
	log.Println("💯 100% delivery guarantee - state persists in queue")
	log.Println("🔄 Resilient to pod crashes and restarts")
	log.Println("🌉 Integrated with Container C for external operations")

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("\n✅ Order processor shutting down")
}