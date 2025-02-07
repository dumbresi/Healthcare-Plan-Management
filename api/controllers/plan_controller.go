package controllers

import (
	"context"
	"encoding/json"
	"github.com/dumbresi/Healthcare-Plan-Management/api/config"
	"github.com/dumbresi/Healthcare-Plan-Management/api/models"
	"github.com/dumbresi/Healthcare-Plan-Management/api/utils"
	"github.com/gofiber/fiber/v2"
)

var ctx = context.Background()

func CreatePlan(c *fiber.Ctx) error {
	var plan models.Plan

	err := c.BodyParser(&plan)

	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request format"})
	}

	// Validate JSON schema
	valid, err := utils.ValidateJSON(plan, "schemas/plan_schema.json")
	if err != nil || !valid {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON structure"})
	}


	planJSON, _ := json.Marshal(plan)
	err = config.RedisClient.Set(ctx, plan.ObjectId, planJSON, 0).Err()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to store data"})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "Plan created successfully"})
}

func GetPlan(c *fiber.Ctx) error {
	id := c.Params("id")
	val, err := config.RedisClient.Get(ctx, id).Result()

	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Plan not found"})
	}

	var plan models.Plan
	json.Unmarshal([]byte(val), &plan)

	return c.Status(fiber.StatusOK).JSON(plan)
}

func DeletePlan(c *fiber.Ctx) error {
	id := c.Params("id")
	err := config.RedisClient.Del(ctx, id).Err()

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete plan"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Plan deleted successfully"})
}
