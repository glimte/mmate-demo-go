# E-commerce Demo System

This demo demonstrates the key patterns in mmate-go:
- **Bridge Pattern**: Synchronous request-response over async messaging
- **StageFlow Pattern**: 100% delivery guarantee with state embedded in messages
- **Integration Pattern**: Message transformation and legacy system bridging

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Container A: API Gateway                    │
│  - REST API (HTTP :8080)                                            │
│  - Uses SyncAsync Bridge to convert HTTP → Messages → HTTP          │
│  - Waits for responses using correlation IDs                        │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼ (Bridge: sync-over-async)
┌─────────────────────────────────────────────────────────────────────┐
│                    Container B: Order Processor                     │
│  - StageFlow for internal workflow (state in messages)              │
│  - Stages: Validate → Inventory → Payment → Shipping                │
│  - Uses Bridge to call Container C for external operations          │
│  - 100% delivery guarantee - survives crashes and pod restart       │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                        ┌───────────┴───────────┐
                        ▼                       ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   Container C: Integration Service                  │
│  - Message transformation (JSON ↔ XML)                              │
│  - Legacy system integration                                        │
│  - Handles inventory checks and shipping arrangements               │
│  - Protocol bridging for external systems                           │
└─────────────────────────────────────────────────────────────────────┘
```

## Key Patterns Demonstrated

### 1. Bridge Pattern (Container A & B)
The Bridge pattern provides synchronous request-response semantics over asynchronous message queues:

```go
// Container A: API Gateway uses Bridge for sync HTTP responses
reply, err := syncAsyncBridge.RequestCommand(ctx, cmd, 30*time.Second)

// Container B: Order Processor uses Bridge to call Container C
inventoryReply, err := integrationBridge.RequestCommand(ctx, checkInventoryCmd, 10*time.Second)
```

### 2. StageFlow Pattern (Container B)
StageFlow provides 100% delivery guarantee by embedding workflow state IN the message itself:

```go
// State travels WITH the message in the queue
// If pod crashes, workflow resumes from last successful stage
workflow := stageflow.NewWorkflow("order-processing")
    .AddStage("validate", validateStage)      // Internal validation
    .AddStage("inventory", inventoryStage)    // Calls Container C via Bridge
    .AddStage("payment", paymentStage)        // Internal processing
    .AddStage("shipping", shippingStage)      // Calls Container C via Bridge
```

### 3. Integration Pattern (Container C)
Container C demonstrates message transformation and legacy system integration:

```go
// Modern JSON → Legacy XML for inventory system
// Modern JSON → Legacy JSON for shipping system
// Handles protocol differences, data mapping, and enrichment
```

## Message Flow Example

1. **HTTP POST** `/api/orders` → Container A
2. Container A creates `ProcessOrderCommand` with correlation ID
3. Container A sends command to Container B using Bridge (waits for response)
4. Container B receives command and starts StageFlow workflow:
   - **Stage 1**: Validate order (internal)
   - **Stage 2**: Check inventory - sends `CheckInventoryCommand` to Container C via Bridge
   - Container C transforms to XML, calls legacy system, returns `InventoryCheckedReply`
   - **Stage 3**: Process payment (internal, simulated)
   - **Stage 4**: Arrange shipping - sends `ArrangeShippingCommand` to Container C via Bridge
   - Container C transforms to legacy JSON, calls system, returns `ShippingArrangedReply`
5. Container B sends `OrderProcessedReply` back to Container A
6. Container A returns HTTP response with tracking number

## Running the Demo

```bash
# Start all services
docker-compose up

# Test successful order processing
curl -X POST http://localhost:8080/api/orders \
  -H "Content-Type: application/json" \
  -d '{
    "customerId": "customer-123",
    "customerName": "John Doe",
    "email": "john@example.com",
    "items": [
      {
        "productId": "prod-001",
        "productName": "Laptop",
        "quantity": 1,
        "price": 999.99
      }
    ]
  }'

# Expected response (immediate - workflow runs asynchronously)
{
  "message": "Order processing started - workflow ID: abc123-def456-...",
  "orderId": "ORD-abc123",
  "processedAt": "2025-06-30T15:30:00Z",
  "status": "processing"
}
```

### Test Queue-Based Crash Recovery

Test the 100% delivery guarantee by restarting Container B during workflow processing:

```bash
# Start order and immediately restart container during processing
curl -X POST http://localhost:8080/api/orders \
  -H "Content-Type: application/json" \
  -d '{
    "customerId": "crash-test",
    "customerName": "Crash Test User",
    "email": "crash@test.com",
    "items": [
      {
        "productId": "crash-prod",
        "productName": "Crash Test Product",
        "quantity": 1,
        "price": 199.99
      }
    ]
  }' && sleep 2 && docker compose restart container-b

# Check logs to verify workflow resumed after restart
docker compose logs container-b --tail=15

# You should see the same workflow instance ID continue processing
# after the container restart - proving state persisted in the queue!
```

## Container Details

### Container A: API Gateway
- **Purpose**: Provides HTTP API and converts to async messaging
- **Key Pattern**: SyncAsync Bridge
- **Endpoints**:
  - `POST /api/orders` - Create new order
  - `GET /api/demo` - Get demo order payload
  - `GET /health` - Health check

### Container B: Order Processor
- **Purpose**: Orchestrates order workflow with StageFlow
- **Key Pattern**: StageFlow with embedded state
- **Stages**:
  1. Validate Order - Check customer, items, email
  2. Check Inventory - Calls Container C via Bridge
  3. Process Payment - Simulated payment processing
  4. Arrange Shipping - Calls Container C via Bridge
- **Features**:
  - State persists in message queue
  - Survives pod crashes
  - Resumes from last successful stage

### Container C: Integration Service
- **Purpose**: Bridges modern services with legacy systems
- **Key Pattern**: Message transformation and protocol bridging
- **Operations**:
  - Inventory Check: JSON → XML → Legacy System → XML → JSON
  - Shipping Arrangement: JSON → Legacy JSON → System → JSON
- **Features**:
  - Real-time metrics reporting
  - Multiple format transformations
  - Simulated legacy system delays

## Fault Tolerance Testing

### Test 1: Container Crash During Processing
```bash
# Start order processing
curl -X POST http://localhost:8080/api/orders -d '...' &

# Kill Container B during processing
docker-compose stop container-b

# Restart Container B
docker-compose start container-b

# Order resumes from last checkpoint - state was in the queue!
```

### Test 2: Integration Service Failure
```bash
# Stop Container C
docker-compose stop container-c

# Try to place order - will timeout at inventory stage
curl -X POST http://localhost:8080/api/orders -d '...'

# Start Container C
docker-compose start container-c

# Retry order - now succeeds
```

## Message Examples

### ProcessOrderCommand (A → B)
```json
{
  "type": "ProcessOrderCommand",
  "id": "cmd-123",
  "correlationId": "corr-456",
  "replyTo": "api-gateway-replies",
  "orderId": "ORD-789",
  "customerId": "CUST-123",
  "items": [...],
  "shippingAddress": {...}
}
```

### CheckInventoryCommand (B → C)
```json
{
  "type": "CheckInventoryCommand",
  "id": "inv-check-123",
  "correlationId": "corr-789",
  "replyTo": "order-processor-integration-replies",
  "orderId": "ORD-789",
  "items": [...]
}
```

### Legacy XML Transformation (in Container C)
```xml
<InventoryRequest transactionId="TXN-ORD-789-1234567890">
  <action>RESERVE</action>
  <items>
    <item>
      <sku>PROD-001</sku>
      <quantity>1</quantity>
    </item>
  </items>
</InventoryRequest>
```

## Key Benefits Demonstrated

1. **Synchronous UX**: HTTP clients get immediate responses despite async processing
2. **100% Delivery**: StageFlow state survives crashes - no lost orders
3. **Service Isolation**: Each container handles its own concerns
4. **Legacy Integration**: Modern services work with legacy systems seamlessly
5. **Observable**: Each stage logs progress with clear indicators

## Monitoring the Demo

Watch the logs to see:
- 🚀 Service startup
- 📨 Message receipt
- 🔄 Stage transitions
- 🌉 Bridge operations
- ✅ Successful completions
- ❌ Failures and retries
- 📊 Metrics (Container C shows transformation stats every 30s)

### Log Examples

```
[Container A] 📨 Received order request
[Container A] 📤 Sending order command via Bridge: ORD-abc123
[Container B] 📨 Received order command: ORD-abc123
[Container B] 📋 [Stage: Validate] Processing workflow
[Container B] ✅ Order validation passed
[Container B] 📦 [Stage: Inventory] Processing workflow
[Container B] 🔄 Sending inventory check to integration service
[Container C] 📦 Processing Inventory Check
[Container C] 🔄 Transformed to XML (215 bytes)
[Container C] ✅ Inventory reserved successfully
[Container B] ✅ Inventory available, reservations: map[PROD-001:RES-...]
[Container B] 💳 [Stage: Payment] Processing workflow
[Container B] ✅ Payment processed: $999.99
[Container B] 🚚 [Stage: Shipping] Processing workflow
[Container C] 🚚 Processing Shipping Arrangement
[Container C] 🔄 Transformed to JSON (284 bytes)
[Container C] ✅ Shipping arranged successfully
[Container B] ✅ Shipping arranged: 1Z123456789 (via FedEx)
[Container B] ✅ Workflow completed successfully
[Container A] ✅ Received reply for order: ORD-abc123
```

This demo shows how mmate-go enables building resilient, distributed systems with enterprise-grade reliability patterns.