package utils

import (
	"github.com/xeipuuv/gojsonschema"
)

func ValidateJSON(data interface{}, schemaPath string) (bool, error) {
	schemaLoader := gojsonschema.NewReferenceLoader("file://" + schemaPath)
	documentLoader := gojsonschema.NewGoLoader(data)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return false, err
	}

	return result.Valid(), nil
}
