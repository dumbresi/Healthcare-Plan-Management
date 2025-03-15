package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"crypto/sha256"
	"github.com/dumbresi/Healthcare-Plan-Management/api/config"
	"github.com/dumbresi/Healthcare-Plan-Management/api/models"
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

	// Get existing plan data from Redis
	val, err := config.RedisClient.Get(ctx, id).Result()
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Plan not found"})
	}

	// Get stored ETag
	storedETag, err := config.RedisClient.Get(ctx, id+":etag").Result()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve ETag"})
	}

	// Check If-Match header for conditional update
	if ifMatch := c.Get("If-Match"); ifMatch != "" && ifMatch != storedETag {
		return c.Status(fiber.StatusPreconditionFailed).JSON(fiber.Map{"error": "Plan has been modified"})
	}

	// Unmarshal the existing plan
	var existingPlan models.Plan
	if err := json.Unmarshal([]byte(val), &existingPlan); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to parse existing plan"})
	}

	// Parse update data from request body
	var updatePlan models.Plan
	if err := c.BodyParser(&updatePlan); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request format"})
	}

	// Validate that the provided ObjectId (if any) matches the existing plan
	if updatePlan.ObjectId != "" && existingPlan.ObjectId != updatePlan.ObjectId {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ObjectId mismatch in plan"})
	}

	// Update PlanCostShares if provided
	if updatePlan.PlanCostShares != nil {
		if existingPlan.PlanCostShares != nil {
			// Validate ObjectId for PlanCostShares if provided
			if updatePlan.PlanCostShares.ObjectId != "" && existingPlan.PlanCostShares.ObjectId != updatePlan.PlanCostShares.ObjectId {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ObjectId mismatch in planCostShares"})
			}

			// Update individual fields.
			// Note: Using non-pointer ints means you cannot update to zero.
			// Consider using *int in an update-specific struct if zero is a valid value.
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

			// Marshal and store the updated PlanCostShares
			costSharesJSON, err := json.Marshal(existingPlan.PlanCostShares)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to marshal updated PlanCostShares"})
			}
			if err := config.RedisClient.Set(ctx, existingPlan.PlanCostShares.ObjectId, costSharesJSON, 0).Err(); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update PlanCostShares"})
			}
		} else {
			// No existing PlanCostShares; assign the new one and store it.
			existingPlan.PlanCostShares = updatePlan.PlanCostShares
			costSharesJSON, err := json.Marshal(existingPlan.PlanCostShares)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to marshal new PlanCostShares"})
			}
			if err := config.RedisClient.Set(ctx, existingPlan.PlanCostShares.ObjectId, costSharesJSON, 0).Err(); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to store PlanCostShares"})
			}
		}
	}

	// Handle updates for LinkedPlanServices if provided
	if len(updatePlan.LinkedPlanServices) > 0 {
		// Create a map for new LinkedPlanServices for easier lookup by ObjectId
		newServicesMap := make(map[string]models.LinkedPlanService)
		for _, newService := range updatePlan.LinkedPlanServices {
			newServicesMap[newService.ObjectId] = newService
		}

		// Update existing LinkedPlanServices if found in new update
		for i, existingService := range existingPlan.LinkedPlanServices {
			if newService, ok := newServicesMap[existingService.ObjectId]; ok {
				existingPlan.LinkedPlanServices[i] = newService
				delete(newServicesMap, existingService.ObjectId)

				// Store updated LinkedPlanService
				serviceJSON, err := json.Marshal(newService)
				if err != nil {
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to marshal updated LinkedPlanService"})
				}
				if err := config.RedisClient.Set(ctx, newService.ObjectId, serviceJSON, 0).Err(); err != nil {
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update LinkedPlanService"})
				}

				// Store LinkedService component
				linkedServiceJSON, err := json.Marshal(newService.LinkedService)
				if err != nil {
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to marshal updated LinkedService"})
				}
				if err := config.RedisClient.Set(ctx, newService.LinkedService.ObjectId, linkedServiceJSON, 0).Err(); err != nil {
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update LinkedService"})
				}

				// Store PlanServiceCostShares component
				costSharesJSON, err := json.Marshal(newService.PlanServiceCostShares)
				if err != nil {
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to marshal updated PlanServiceCostShares"})
				}
				if err := config.RedisClient.Set(ctx, newService.PlanServiceCostShares.ObjectId, costSharesJSON, 0).Err(); err != nil {
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update PlanServiceCostShares"})
				}
			}
		}

		// Append any new LinkedPlanServices that did not match existing ones
		for _, newService := range newServicesMap {
			existingPlan.LinkedPlanServices = append(existingPlan.LinkedPlanServices, newService)

			// Store the new LinkedPlanService and its components
			serviceJSON, err := json.Marshal(newService)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to marshal new LinkedPlanService"})
			}
			if err := config.RedisClient.Set(ctx, newService.ObjectId, serviceJSON, 0).Err(); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to store LinkedPlanService"})
			}

			linkedServiceJSON, err := json.Marshal(newService.LinkedService)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to marshal new LinkedService"})
			}
			if err := config.RedisClient.Set(ctx, newService.LinkedService.ObjectId, linkedServiceJSON, 0).Err(); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to store LinkedService"})
			}

			costSharesJSON, err := json.Marshal(newService.PlanServiceCostShares)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to marshal new PlanServiceCostShares"})
			}
			if err := config.RedisClient.Set(ctx, newService.PlanServiceCostShares.ObjectId, costSharesJSON, 0).Err(); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to store PlanServiceCostShares"})
			}
		}
	}

	// Update other simple fields if provided
	if updatePlan.CreationDate != "" {
		existingPlan.CreationDate = updatePlan.CreationDate
	}
	if updatePlan.ObjectType != "" {
		existingPlan.ObjectType = updatePlan.ObjectType
	}
	if updatePlan.Org != "" {
		existingPlan.Org = updatePlan.Org
	}

	// Generate new JSON for the updated plan and create a new ETag
	planJSON, err := json.Marshal(existingPlan)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to marshal updated plan"})
	}
	newETag := fmt.Sprintf("\"%x\"", sha256.Sum256(planJSON))

	// Store updated plan and new ETag in Redis
	if err := config.RedisClient.Set(ctx, id, planJSON, 0).Err(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update plan data"})
	}
	if err := config.RedisClient.Set(ctx, id+":etag", newETag, 0).Err(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update ETag"})
	}

	// Set new ETag in response header and return the updated plan
	c.Set("ETag", newETag)
	return c.Status(fiber.StatusOK).JSON(existingPlan)
}

