package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dumbresi/Healthcare-Plan-Management/api/config"
	"github.com/dumbresi/Healthcare-Plan-Management/api/models"
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

	err := c.BodyParser(&plan)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request format"})
	}

	// ... existing code ...

	planJSON, _ := json.Marshal(plan)

	// Generate ETag (using simple hash of JSON)
	etag := fmt.Sprintf("\"%x\"", sha256.Sum256(planJSON))

	// Store both plan and its ETag
	err = config.RedisClient.Set(ctx, plan.ObjectId, planJSON, 0).Err()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to store data"})
	}

	// Store ETag separately
	err = config.RedisClient.Set(ctx, plan.ObjectId+":etag", etag, 0).Err()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to store ETag"})
	}

	// Set ETag in response header
	c.Set("ETag", etag)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "Plan created successfully"})
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
	err := config.RedisClient.Del(ctx, id).Err()

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete plan"})
	}

	return c.Status(fiber.StatusNoContent).JSON(fiber.Map{"message": "Plan deleted successfully"})
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
	if ifMatch != storedETag && ifMatch!="\""+storedETag+"\"" {
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
	return c.Status(fiber.StatusOK).JSON(existingPlan)
}
