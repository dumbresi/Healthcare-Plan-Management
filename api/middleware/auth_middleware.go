package middleware

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/joho/godotenv"
)

const (
	googleCertsURL = "https://www.googleapis.com/oauth2/v3/certs"
	googleIssuer   = "https://accounts.google.com"
)

var (
	certsCache     googleCerts
	certsCacheTime time.Time
	cacheMutex     sync.Mutex
)

// Struct to hold Google's public keys
type googleCerts struct {
	Keys []struct {
		Kid string `json:"kid"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

// Fetch Google’s public keys (cached for performance)
func getGooglePublicKeys() (*googleCerts, error) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	if time.Since(certsCacheTime) < time.Hour {
		return &certsCache, nil
	}

	resp, err := http.Get(googleCertsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var certs googleCerts
	if err := json.Unmarshal(body, &certs); err != nil {
		return nil, err
	}

	certsCache = certs
	certsCacheTime = time.Now()
	return &certs, nil
}

// Convert Google's key format to *rsa.PublicKey
func getPublicKey(kid string) (*rsa.PublicKey, error) {
	certs, err := getGooglePublicKeys()
	if err != nil {
		return nil, err
	}

	for _, key := range certs.Keys {
		if key.Kid == kid {
			nBytes, _ := jwt.DecodeSegment(key.N)
			eBytes, _ := jwt.DecodeSegment(key.E)

			n := new(big.Int).SetBytes(nBytes)
			e := int(new(big.Int).SetBytes(eBytes).Int64())

			pubKey := &rsa.PublicKey{N: n, E: e}
			return pubKey, nil
		}
	}
	return nil, errors.New("public key not found")
}

// Verify Google JWT token
func verifyGoogleToken(tokenString string) (*jwt.MapClaims, error) {
	err := godotenv.Load(".env")
	if err != nil {
    	log.Print("error loading env")
    }
	var cliendID=os.Getenv("CLIENT_ID")

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("missing kid")
		}

		return getPublicKey(kid)
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if claims["iss"] != googleIssuer {
			return nil, errors.New("invalid issuer")
		}

		if claims["aud"] != cliendID {
			return nil, errors.New("invalid audience")
		}

		return &claims, nil
	}

	return nil, errors.New("invalid token")
}

// Fiber middleware for authentication
func AuthMiddleware(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing Authorization header"})
	}

	tokenParts := strings.Split(authHeader, " ")
	if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid Authorization format"})
	}

	token := tokenParts[1]
	userClaims, err := verifyGoogleToken(token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": fmt.Sprintf("Unauthorized: %v", err)})
	}

	// Store user claims in Fiber's Locals (accessible in handlers)
	c.Locals("user", userClaims)
	return c.Next()
}
