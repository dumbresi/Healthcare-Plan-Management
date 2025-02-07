package models

type Plan struct {
	PlanCostShares     *PlanCostShares        `json:"planCostShares,omitempty"`
	LinkedPlanServices []LinkedPlanService    `json:"linkedPlanServices,omitempty"`
	PlanStatus         string                 `json:"planStatus" binding:"required"`
	CreationDate       string                 `json:"creationDate" binding:"required"`
	ObjectId           string                 `json:"objectId" binding:"required"`
	ObjectType         string                 `json:"objectType" binding:"required"`
	Org                string                 `json:"_org" binding:"required"`
	PlanJoin           map[string]interface{} `json:"plan_join,omitempty"`
}

type PlanCostShares struct {
	Deductible int                    `json:"deductible" binding:"required"`
	Copay      int                    `json:"copay" binding:"required"`
	ObjectId   string                 `json:"objectId" binding:"required"`
	ObjectType string                 `json:"objectType" binding:"required"`
	Org        string                 `json:"_org" binding:"required"`
	PlanJoin   map[string]interface{} `json:"plan_join,omitempty"`
}

type LinkedService struct {
	Name       string                 `json:"name" binding:"required"`
	ObjectId   string                 `json:"objectId" binding:"required"`
	ObjectType string                 `json:"objectType" binding:"required"`
	Org        string                 `json:"_org" binding:"required"`
	PlanJoin   map[string]interface{} `json:"plan_join,omitempty"`
}

type PlanServiceCostShares struct {
	Deductible int                    `json:"deductible" binding:"required"`
	Copay      int                    `json:"copay" binding:"required"`
	ObjectId   string                 `json:"objectId" binding:"required"`
	ObjectType string                 `json:"objectType" binding:"required"`
	Org        string                 `json:"_org" binding:"required"`
	PlanJoin   map[string]interface{} `json:"plan_join,omitempty"`
}

type LinkedPlanService struct {
	LinkedService         LinkedService          `json:"linkedService" binding:"required"`
	PlanServiceCostShares PlanServiceCostShares  `json:"planserviceCostShares" binding:"required"`
	ObjectId              string                 `json:"objectId" binding:"required"`
	ObjectType            string                 `json:"objectType" binding:"required"`
	Org                   string                 `json:"_org" binding:"required"`
	PlanJoin   map[string]interface{} `json:"plan_join,omitempty"`
}

