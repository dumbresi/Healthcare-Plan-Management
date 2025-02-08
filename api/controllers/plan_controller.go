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
	
	// Get ETag from request header
	requestETag := c.Get("ETag")
	if requestETag == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ETag header is required"})
	}

	// Get the stored ETag for validation
	storedETag, err := config.RedisClient.Get(ctx, id+":etag").Result()
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Plan not found"})
	}

	// Verify if the provided ETag matches the stored one
	if requestETag != storedETag {
		return c.Status(fiber.StatusPreconditionFailed).JSON(fiber.Map{"error": "ETag mismatch"})
	}

	// Check If-None-Match header for conditional read
	if ifNoneMatch := c.Get("If-None-Match"); ifNoneMatch == storedETag {
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
