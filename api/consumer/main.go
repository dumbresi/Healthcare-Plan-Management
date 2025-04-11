package main

import (
	"bytes"
	"encoding/json"
	"log"

	"github.com/dumbresi/Healthcare-Plan-Management/api/models"
	"github.com/elastic/go-elasticsearch/v8"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	log.Println("Starting to consume messages from the queue")

	// Connect to RabbitMQ
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	queue, err := ch.QueueDeclare(
		"plans_queue", // name
		false,         // durable
		false,         // delete when unused
		false,         // exclusive
		false,         // no-wait
		nil,           // arguments
	)
	failOnError(err, "Failed to declare a queue")

	msgs, err := ch.Consume(
		queue.Name,      // queue
		"plansConsumer", // consumer
		true,            // auto-ack
		false,           // exclusive
		false,           // no-local
		false,           // no-wait
		nil,             // args
	)
	failOnError(err, "Failed to register a consumer")

	// Connect to Elasticsearch
	cfg := elasticsearch.Config{
		Addresses: []string{
			"http://localhost:9200",
		},
	}
	es, err := elasticsearch.NewClient(cfg)
	failOnError(err, "Failed to create the Elasticsearch client")

	res, err := es.Indices.Create("plans")
	if err != nil {
		log.Fatalf("Error getting response: %s", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		log.Printf("Error creating the index: %s", res.String())
	} else {
		log.Printf("Index 'plans' created successfully")
	}

	// Put Mapping
	jsonData, err := json.Marshal(getMapping())
	failOnError(err, "Failed to serialize the mapping")

	res, err = es.Indices.PutMapping(
		[]string{"plans"},
		bytes.NewReader(jsonData),
	)
	if err != nil {
		log.Fatalf("Error getting response: %s", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		log.Printf("Error applying mapping: %s", res.String())
	} else {
		log.Printf("Mapping applied successfully")
	}

	forever := make(chan bool)

	go func() {
		for d := range msgs {
			log.Printf("Received a message: %s", d.Body)

			// Deserialize the PlanMessage
			var planMessage models.PlanMessage
			err := json.Unmarshal(d.Body, &planMessage)
			failOnError(err, "Failed to deserialize PlanMessage")

			switch planMessage.Operation {
			case "create":
				handleCreateOperation(es, planMessage.Plan)
			case "patch":
				handleCreateOperation(es, planMessage.Plan)
			case "delete":
				handleDeleteOperation(es, planMessage.Plan)
			default:
				log.Printf("Unknown operation: %s", planMessage.Operation)
			}
		}
	}()

	log.Printf(" [*] Waiting for messages. To exit press CTRL+C")
	<-forever
}

func handleCreateOperation(es *elasticsearch.Client, plan models.Plan) {
	// Add the plan_join field to the plan object
	plan.PlanJoin = map[string]interface{}{
		"name": "plan",
	}

	// Serialize the plan object with the added plan_join fields
	planJSON, err := json.Marshal(plan)
	failOnError(err, "Failed to serialize Plan object")

	// Index the plan document
	res, err := es.Index(
		"plans",
		bytes.NewReader(planJSON),
		es.Index.WithDocumentID(plan.ObjectId),
		es.Index.WithRefresh("true"),
	)
	if err != nil {
		log.Fatalf("Error getting response: %s", err)
	}
	if res.IsError() {
		log.Printf("Error indexing document ID=%s: %s", plan.ObjectId, res.String())
	} else {
		log.Printf("Successfully indexed document ID=%s", plan.ObjectId)
	}

	// Index the planCostShares document
	plan.PlanCostShares.PlanJoin = map[string]interface{}{
		"name":   "planCostShares",
		"parent": plan.ObjectId,
	}

	// Serialize the planCostShares object with the added plan_join fields
	planCostSharesJSON, err := json.Marshal(plan.PlanCostShares)
	failOnError(err, "Failed to serialize PlanCostShares object")

	// Index the planCostShares document
	res, err = es.Index(
		"plans",
		bytes.NewReader(planCostSharesJSON),
		es.Index.WithDocumentID(plan.PlanCostShares.ObjectId),
		es.Index.WithRouting(plan.ObjectId),
		es.Index.WithRefresh("true"),
	)
	if err != nil {
		log.Fatalf("Error getting response: %s", err)
	}
	if res.IsError() {
		log.Printf("Error indexing document ID=%s: %s", plan.PlanCostShares.ObjectId, res.String())
	} else {
		log.Printf("Successfully indexed document ID=%s", plan.PlanCostShares.ObjectId)
	}

	// Index each linkedPlanServices document
	for _, linkedPlanService := range plan.LinkedPlanServices {
		linkedPlanService.PlanJoin = map[string]interface{}{
			"name":   "linkedPlanServices",
			"parent": plan.ObjectId,
		}

		// Serialize the linkedPlanServices object with the added plan_join fields
		linkedPlanServiceJSON, err := json.Marshal(linkedPlanService)
		failOnError(err, "Failed to serialize LinkedPlanService object")

		// Index the linkedPlanServices document
		res, err = es.Index(
			"plans",
			bytes.NewReader(linkedPlanServiceJSON),
			es.Index.WithDocumentID(linkedPlanService.ObjectId),
			es.Index.WithRouting(plan.ObjectId),
			es.Index.WithRefresh("true"),
		)
		if err != nil {
			log.Fatalf("Error getting response: %s", err)
		}
		if res.IsError() {
			log.Printf("Error indexing document ID=%s: %s", linkedPlanService.ObjectId, res.String())
		} else {
			log.Printf("Successfully indexed document ID=%s", linkedPlanService.ObjectId)
		}

		// Index the linkedService document
		linkedPlanService.LinkedService.PlanJoin = map[string]interface{}{
			"name":   "linkedService",
			"parent": linkedPlanService.ObjectId,
		}

		// Serialize the linkedService object with the added plan_join fields
		linkedServiceJSON, err := json.Marshal(linkedPlanService.LinkedService)
		failOnError(err, "Failed to serialize LinkedService object")

		// Index the linkedService document
		res, err = es.Index(
			"plans",
			bytes.NewReader(linkedServiceJSON),
			es.Index.WithDocumentID(linkedPlanService.LinkedService.ObjectId),
			es.Index.WithRouting(linkedPlanService.ObjectId),
			es.Index.WithRefresh("true"),
		)
		if err != nil {
			log.Fatalf("Error getting response: %s", err)
		}
		if res.IsError() {
			log.Printf("Error indexing document ID=%s: %s", linkedPlanService.LinkedService.ObjectId, res.String())
		} else {
			log.Printf("Successfully indexed document ID=%s", linkedPlanService.LinkedService.ObjectId)
		}

		// Index the planserviceCostShares document
		linkedPlanService.PlanServiceCostShares.PlanJoin = map[string]interface{}{
			"name":   "planserviceCostShares",
			"parent": linkedPlanService.ObjectId,
		}

		// Serialize the planserviceCostShares object with the added plan_join fields
		planServiceCostSharesJSON, err := json.Marshal(linkedPlanService.PlanServiceCostShares)
		failOnError(err, "Failed to serialize PlanServiceCostShares object")

		// Index the planserviceCostShares document
		res, err = es.Index(
			"plans",
			bytes.NewReader(planServiceCostSharesJSON),
			es.Index.WithDocumentID(linkedPlanService.PlanServiceCostShares.ObjectId),
			es.Index.WithRouting(linkedPlanService.ObjectId),
			es.Index.WithRefresh("true"),
		)
		if err != nil {
			log.Fatalf("Error getting response: %s", err)
		}
		if res.IsError() {
			log.Printf("Error indexing document ID=%s: %s", linkedPlanService.PlanServiceCostShares.ObjectId, res.String())
		} else {
			log.Printf("Successfully indexed document ID=%s", linkedPlanService.PlanServiceCostShares.ObjectId)
		}
	}
}

func handleDeleteOperation(es *elasticsearch.Client, plan models.Plan) {
	// Delete the main plan document
	res, err := es.Delete("plans", plan.ObjectId)
	if err != nil {
		log.Fatalf("Error deleting plan: %s", err)
	}
	if res.IsError() {
		log.Printf("Error deleting plan ID=%s: %s", plan.ObjectId, res.String())
	} else {
		log.Printf("Successfully deleted plan ID=%s", plan.ObjectId)
	}

	// Delete planCostShares document
	res, err = es.Delete("plans", plan.PlanCostShares.ObjectId)
	if err != nil {
		log.Fatalf("Error deleting planCostShares: %s", err)
	}
	if res.IsError() {
		log.Printf("Error deleting planCostShares ID=%s: %s", plan.PlanCostShares.ObjectId, res.String())
	} else {
		log.Printf("Successfully deleted planCostShares ID=%s", plan.PlanCostShares.ObjectId)
	}

	// Delete linkedPlanServices and their linkedService documents
	for _, linkedPlanService := range plan.LinkedPlanServices {
		// Delete linkedPlanService
		res, err := es.Delete("plans", linkedPlanService.ObjectId)
		if err != nil {
			log.Fatalf("Error deleting linkedPlanService: %s", err)
		}
		if res.IsError() {
			log.Printf("Error deleting linkedPlanService ID=%s: %s", linkedPlanService.ObjectId, res.String())
		} else {
			log.Printf("Successfully deleted linkedPlanService ID=%s", linkedPlanService.ObjectId)
		}

		// Delete linkedService
		res, err = es.Delete("plans", linkedPlanService.LinkedService.ObjectId)
		if err != nil {
			log.Fatalf("Error deleting linkedService: %s", err)
		}
		if res.IsError() {
			log.Printf("Error deleting linkedService ID=%s: %s", linkedPlanService.LinkedService.ObjectId, res.String())
		} else {
			log.Printf("Successfully deleted linkedService ID=%s", linkedPlanService.LinkedService.ObjectId)
		}

		// Delete planserviceCostShares
		res, err = es.Delete("plans", linkedPlanService.PlanServiceCostShares.ObjectId)
		if err != nil {
			log.Fatalf("Error deleting planServiceCostShares: %s", err)
		}
		if res.IsError() {
			log.Printf("Error deleting planServiceCostShares ID=%s: %s", linkedPlanService.PlanServiceCostShares.ObjectId, res.String())
		} else {
			log.Printf("Successfully deleted planServiceCostShares ID=%s", linkedPlanService.PlanServiceCostShares.ObjectId)
		}
	}
}

func getMapping() map[string]interface{} {
	return map[string]interface{}{
		"properties": map[string]interface{}{
			"plan": map[string]interface{}{
				"properties": map[string]interface{}{
					"_org": map[string]interface{}{
						"type": "text",
					},
					"objectId": map[string]interface{}{
						"type": "keyword",
					},
					"objectType": map[string]interface{}{
						"type": "text",
					},
					"planType": map[string]interface{}{
						"type": "text",
					},
					"creationDate": map[string]interface{}{
						"type":   "date",
						"format": "MM-dd-yyyy",
					},
				},
			},
			"planCostShares": map[string]interface{}{
				"properties": map[string]interface{}{
					"copay": map[string]interface{}{
						"type": "long",
					},
					"deductible": map[string]interface{}{
						"type": "long",
					},
					"_org": map[string]interface{}{
						"type": "text",
					},
					"objectId": map[string]interface{}{
						"type": "keyword",
					},
					"objectType": map[string]interface{}{
						"type": "text",
					},
				},
			},
			"linkedPlanServices": map[string]interface{}{
				"properties": map[string]interface{}{
					"_org": map[string]interface{}{
						"type": "text",
					},
					"objectId": map[string]interface{}{
						"type": "keyword",
					},
					"objectType": map[string]interface{}{
						"type": "text",
					},
				},
			},
			"linkedService": map[string]interface{}{
				"properties": map[string]interface{}{
					"_org": map[string]interface{}{
						"type": "text",
					},
					"name": map[string]interface{}{
						"type": "text",
					},
					"objectId": map[string]interface{}{
						"type": "keyword",
					},
					"objectType": map[string]interface{}{
						"type": "text",
					},
				},
			},
			"planserviceCostShares": map[string]interface{}{
				"properties": map[string]interface{}{
					"copay": map[string]interface{}{
						"type": "long",
					},
					"deductible": map[string]interface{}{
						"type": "long",
					},
					"_org": map[string]interface{}{
						"type": "text",
					},
					"objectId": map[string]interface{}{
						"type": "keyword",
					},
					"objectType": map[string]interface{}{
						"type": "text",
					},
				},
			},
			"plan_join": map[string]interface{}{
				"type":                  "join",
				"eager_global_ordinals": "true",
				"relations": map[string]interface{}{
					"plan":               []string{"planCostShares", "linkedPlanServices"},
					"linkedPlanServices": []string{"linkedService", "planserviceCostShares"},
				},
			},
		},
	}
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}
