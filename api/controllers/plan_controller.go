package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/dumbresi/Healthcare-Plan-Management/api/config"
	"github.com/dumbresi/Healthcare-Plan-Management/api/models"
	"github.com/dumbresi/Healthcare-Plan-Management/api/rabbitmq"
	"github.com/redis/go-redis/v9"

	// "github.com/dumbresi/Healthcare-Plan-Management/api/utils"
	"github.com/gofiber/fiber/v2"
)

var ctx = context.Background()

func GetAllPlans(c *fiber.Ctx) error {
	// Use Redis SCAN to iterate through keys
	var cursor uint64
	var allPlans []models.Plan

	// We'll collect all keys first, then filter out the etag keys
	for {
		// Scan with pattern match to avoid etag keys
		keys, nextCursor, err := config.RedisClient.Scan(ctx, cursor, "*", 100).Result()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve plans"})
		}

		// Process each key
		for _, key := range keys {
			// Skip etag keys
			if len(key) > 5 && key[len(key)-5:] == ":etag" {
				continue
			}

			// Get the plan data
			val, err := config.RedisClient.Get(ctx, key).Result()
			if err != nil {
				continue // Skip if there's an error getting this plan
			}

			var plan models.Plan
			if err := json.Unmarshal([]byte(val), &plan); err != nil {
				continue // Skip if it's not a valid plan
			}

			// Only include documents that are actually plans (they have an ObjectType)
			if plan.ObjectType == "plan" {
				allPlans = append(allPlans, plan)
			}
		}

		// Break if we've completed the scan
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return c.Status(fiber.StatusOK).JSON(allPlans)
}

func CreatePlan(c *fiber.Ctx) error {
	var plan models.Plan

	// Step 1: Parse JSON from request body
	if err := c.BodyParser(&plan); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid JSON format",
			"details": err.Error(),
		})
	}

	// Step 2: Basic Validation
	if plan.ObjectId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"field":   "objectId",
			"message": "ObjectId is required",
		})
	}
	if plan.PlanCostShares == nil || plan.PlanCostShares.ObjectId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"field":   "planCostShares.objectId",
			"message": "PlanCostShares and its ObjectId are required",
		})
	}
	if len(plan.LinkedPlanServices) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"field":   "linkedPlanServices",
			"message": "At least one LinkedPlanService is required",
		})
	}
	for i, service := range plan.LinkedPlanServices {
		if service.ObjectId == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Validation failed",
				"field":   fmt.Sprintf("linkedPlanServices[%d].objectId", i),
				"message": "LinkedPlanService ObjectId is required",
			})
		}
		if service.LinkedService.ObjectId == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Validation failed",
				"field":   fmt.Sprintf("linkedPlanServices[%d].linkedService.objectId", i),
				"message": "LinkedService ObjectId is required",
			})
		}
		if service.PlanServiceCostShares.ObjectId == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Validation failed",
				"field":   fmt.Sprintf("linkedPlanServices[%d].planserviceCostShares.objectId", i),
				"message": "PlanServiceCostShares ObjectId is required",
			})
		}
	}

	// Step 3: Check if plan already exists
	exists, err := config.RedisClient.Exists(ctx, plan.ObjectId).Result()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to check plan existence",
			"details": err.Error(),
		})
	}
	if exists == 1 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error":    "Plan already exists",
			"objectId": plan.ObjectId,
		})
	}

	// Step 4: Marshal full plan and subcomponents with error debug logs
	planJSON, err := json.Marshal(plan)
	if err != nil {
		log.Printf("Failed to marshal full plan: %v", err)

		if plan.PlanCostShares != nil {
			if _, err2 := json.Marshal(plan.PlanCostShares); err2 != nil {
				log.Printf("Failed to marshal PlanCostShares: %v", err2)
			}
		}

		for i, lps := range plan.LinkedPlanServices {
			if _, err := json.Marshal(lps); err != nil {
				log.Printf("Failed to marshal LinkedPlanService[%d]: %v", i, err)
			}
			if _, err := json.Marshal(lps.LinkedService); err != nil {
				log.Printf("Failed to marshal LinkedService[%d]: %v", i, err)
			}
			if _, err := json.Marshal(lps.PlanServiceCostShares); err != nil {
				log.Printf("Failed to marshal PlanServiceCostShares[%d]: %v", i, err)
			}
		}

		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Failed to marshal plan object",
			"details": err.Error(),
		})
	}

	// Step 5: Generate ETag from hash
	etag := fmt.Sprintf("\"%x\"", sha256.Sum256(planJSON))

	// Step 6: Store plan and ETag in Redis
	if err := config.RedisClient.Set(ctx, plan.ObjectId, planJSON, 0).Err(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to store plan in Redis"})
	}
	if err := config.RedisClient.Set(ctx, plan.ObjectId+":etag", etag, 0).Err(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to store ETag"})
	}

	// Step 7: Set ETag in response header
	c.Set("ETag", etag)

	msg := models.PlanMessage{
		Operation: "create",
		Plan:      plan,
	}
	rmq := &rabbitmq.Factory{}
	if err := rmq.PublishMessage("plans_queue", msg); err != nil {
	log.Printf("Failed to publish RabbitMQ message: %v", err)
	// Don't fail the request, but log it (or return 202 Accepted if you want async behavior)
	}

	// Step 8: Respond success
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":  "Plan created successfully",
		"objectId": plan.ObjectId,
	})
}


func GetPlan(c *fiber.Ctx) error {
	id := c.Params("id")

	// Get the stored ETag
	storedETag, err := config.RedisClient.Get(ctx, id+":etag").Result()
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Plan not found"})
	}

	// Check If-None-Match header for conditional read
	if ifNoneMatch := c.Get("If-None-Match"); ifNoneMatch != "" && ifNoneMatch == storedETag {
		return c.SendStatus(fiber.StatusNotModified)
	}

	val, err := config.RedisClient.Get(ctx, id).Result()
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Plan not found"})
	}

	var plan models.Plan
	json.Unmarshal([]byte(val), &plan)

	// Set ETag in response header
	c.Set("ETag", storedETag)
	return c.Status(fiber.StatusOK).JSON(plan)
}

func DeletePlan(c *fiber.Ctx) error {
    id := c.Params("id")

    // Check if plan exists and get the plan data
    val, err := config.RedisClient.Get(ctx, id).Result()
    if err == redis.Nil {
        return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Plan not found"})
    } else if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
            "error": "Failed to fetch plan",
            "details": err.Error(),
        })
    }

    // Unmarshal the plan to get child object IDs
    var plan models.Plan
    if err := json.Unmarshal([]byte(val), &plan); err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
            "error": "Failed to unmarshal plan data",
            "details": err.Error(),
        })
    }

    // Collect all keys to delete
    keysToDelete := []string{
        id,                // Main plan
        id + ":etag",     // Plan's ETag
    }

    // Add PlanCostShares keys
    if plan.PlanCostShares != nil {
        keysToDelete = append(keysToDelete, 
            plan.PlanCostShares.ObjectId,
            plan.PlanCostShares.ObjectId + ":etag",
        )
    }

    // Add LinkedPlanServices and their child objects
    for _, service := range plan.LinkedPlanServices {
        // Add LinkedPlanService keys
        keysToDelete = append(keysToDelete,
            service.ObjectId,
            service.ObjectId + ":etag",
        )

        // Add LinkedService keys
        keysToDelete = append(keysToDelete,
            service.LinkedService.ObjectId,
            service.LinkedService.ObjectId + ":etag",
        )

        // Add PlanServiceCostShares keys
        keysToDelete = append(keysToDelete,
            service.PlanServiceCostShares.ObjectId,
            service.PlanServiceCostShares.ObjectId + ":etag",
        )
    }

    // Use Redis Pipeline to delete all keys atomically
    pipe := config.RedisClient.Pipeline()
    for _, key := range keysToDelete {
        pipe.Del(ctx, key)
    }

    // Execute the pipeline
    if _, err := pipe.Exec(ctx); err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
            "error": "Failed to delete plan and its components",
            "details": err.Error(),
        })
    }

    // Publish delete message to RabbitMQ
    msg := models.PlanMessage{
        Operation: "delete",
        Plan:      plan,
    }
    rmq := &rabbitmq.Factory{}
    if err := rmq.PublishMessage("plans_queue", msg); err != nil {
        log.Printf("Failed to publish delete message: %v", err)
    }

    return c.Status(fiber.StatusOK).JSON(fiber.Map{
        "message": "Plan and all related components deleted successfully",
        "deletedKeys": keysToDelete,
    })
}

func PatchPlan(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := context.Background()

	// Retrieve existing plan from Redis
	val, err := config.RedisClient.Get(ctx, id).Result()
	if err == redis.Nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Plan not found"})
	} else if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve plan"})
	}

	// Retrieve stored ETag
	storedETag, err := config.RedisClient.Get(ctx, id+":etag").Result()
	storedETag = strings.Trim(storedETag, "\"")
	// fmt.Print(storedETag)
	if err == redis.Nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "ETag not found"})
	} else if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve ETag"})
	}

	// Enforce If-Match header
	ifMatch := c.Get("If-Match")
	if ifMatch == "" {
		return c.Status(fiber.StatusPreconditionRequired).JSON(fiber.Map{"error": "If-Match header is required"})
	}
	if ifMatch != storedETag && ifMatch != "\""+storedETag+"\"" {
		return c.Status(fiber.StatusPreconditionFailed).JSON(fiber.Map{"error": "Plan has been modified, update aborted"})
	}

	// Parse existing plan
	var existingPlan models.Plan
	if err := json.Unmarshal([]byte(val), &existingPlan); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to parse existing plan"})
	}

	// Parse incoming update data
	var updatePlan models.Plan
	if err := c.BodyParser(&updatePlan); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request format"})
	}

	// Validate ObjectId consistency
	if updatePlan.ObjectId != "" && updatePlan.ObjectId != existingPlan.ObjectId {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ObjectId mismatch in plan"})
	}

	// Apply updates only to provided fields
	if updatePlan.PlanCostShares != nil {
		if existingPlan.PlanCostShares != nil {
			if existingPlan.PlanCostShares.ObjectId != updatePlan.PlanCostShares.ObjectId {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ObjectId mismatch in PlanCostShares"})
			}
			// Apply non-zero updates
			if updatePlan.PlanCostShares.Deductible != 0 {
				existingPlan.PlanCostShares.Deductible = updatePlan.PlanCostShares.Deductible
			}
			if updatePlan.PlanCostShares.Copay != 0 {
				existingPlan.PlanCostShares.Copay = updatePlan.PlanCostShares.Copay
			}
			if updatePlan.PlanCostShares.ObjectType != "" {
				existingPlan.PlanCostShares.ObjectType = updatePlan.PlanCostShares.ObjectType
			}
			if updatePlan.PlanCostShares.Org != "" {
				existingPlan.PlanCostShares.Org = updatePlan.PlanCostShares.Org
			}
		} else {
			existingPlan.PlanCostShares = updatePlan.PlanCostShares
		}
	}

	// Handle LinkedPlanServices updates
	if len(updatePlan.LinkedPlanServices) > 0 {
		updatedServices := make(map[string]models.LinkedPlanService)
		for _, newService := range updatePlan.LinkedPlanServices {
			updatedServices[newService.ObjectId] = newService
		}

		for i, existingService := range existingPlan.LinkedPlanServices {
			if newService, exists := updatedServices[existingService.ObjectId]; exists {
				existingPlan.LinkedPlanServices[i] = newService
				delete(updatedServices, existingService.ObjectId)
			}
		}

		for _, newService := range updatedServices {
			existingPlan.LinkedPlanServices = append(existingPlan.LinkedPlanServices, newService)
		}
	}

	// Update other primitive fields
	if updatePlan.CreationDate != "" {
		existingPlan.CreationDate = updatePlan.CreationDate
	}
	if updatePlan.ObjectType != "" {
		existingPlan.ObjectType = updatePlan.ObjectType
	}
	if updatePlan.Org != "" {
		existingPlan.Org = updatePlan.Org
	}

	// Generate new ETag
	updatedPlanJSON, _ := json.Marshal(existingPlan)
	newETag := fmt.Sprintf("\"%x\"", sha256.Sum256(updatedPlanJSON))

	// Atomic Redis transaction
	pipe := config.RedisClient.TxPipeline()
	pipe.Set(ctx, id, updatedPlanJSON, 0)
	pipe.Set(ctx, id+":etag", newETag, 0)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update plan"})
	}

	// Return updated plan with new ETag
	c.Set("ETag", newETag)

	msg := models.PlanMessage{
		Operation: "patch",
		Plan:      existingPlan,
	}

	rmq := &rabbitmq.Factory{}
	if err := rmq.PublishMessage("plans_queue", msg); err != nil {
		log.Printf("Failed to publish patch message: %v", err)
	}

	return c.Status(fiber.StatusOK).JSON(existingPlan)
}
