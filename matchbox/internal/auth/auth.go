package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var (
	jwtSecret       = []byte(getEnv("JWT_SECRET", "super-secret-key-change-me"))
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

type AuthService struct {
	rdb *redis.Client
}

func NewAuthService(rdb *redis.Client) *AuthService {
	return &AuthService{rdb: rdb}
}

// HashPassword hashes a plain text password using bcrypt.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(bytes), err
}

// CheckPasswordHash compares a hashed password with a plain text one.
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// GenerateTokens creates a short-lived access token and a long-lived refresh token.
// The refresh token is saved to Redis to allow revocation and rotation.
func (s *AuthService) GenerateTokens(ctx context.Context, userID string) (*TokenPair, error) {
	// Access Token (15 mins)
	accessClaims := jwt.MapClaims{
		"sub":  userID,
		"exp":  time.Now().Add(15 * time.Minute).Unix(),
		"iat":  time.Now().Unix(),
		"type": "access",
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessString, err := accessToken.SignedString(jwtSecret)
	if err != nil {
		return nil, err
	}

	// Refresh Token (7 Days)
	refreshUUID := uuid.New().String()
	refreshClaims := jwt.MapClaims{
		"sub":  userID,
		"jti":  refreshUUID,
		"exp":  time.Now().Add(7 * 24 * time.Hour).Unix(),
		"type": "refresh",
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshString, err := refreshToken.SignedString(jwtSecret)
	if err != nil {
		return nil, err
	}

	// Store Refresh Token in Redis (key: refresh_token:<jti>, value: userID)
	// This helps with Revocation and Rotation tracking.
	err = s.rdb.Set(ctx, "refresh_token:"+refreshUUID, userID, 7*24*time.Hour).Err()
	if err != nil {
		return nil, fmt.Errorf("failed to save refresh token in redis: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessString,
		RefreshToken: refreshString,
	}, nil
}

// RotateRefreshToken takes an existing refresh token, validates it against Redis,
// deletes it to prevent reuse (Rotation), and issues a new pair of tokens.
func (s *AuthService) RotateRefreshToken(ctx context.Context, tokenString string) (*TokenPair, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	tokenType, _ := claims["type"].(string)
	if tokenType != "refresh" {
		return nil, errors.New("invalid token type for rotation")
	}

	jti, ok := claims["jti"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}

	userID, ok := claims["sub"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}

	// Verify in Redis & Delete atomically (Rotation)
	val, err := s.rdb.GetDel(ctx, "refresh_token:"+jti).Result()
	if err == redis.Nil {
		// If it's not in Redis, it might have been reused/stolen.
		return nil, errors.New("refresh token already used or revoked")
	} else if err != nil {
		return nil, err
	}

	if val != userID {
		return nil, errors.New("token user mismatch")
	}

	// Issue new token pair
	return s.GenerateTokens(ctx, userID)
}

// AuthMiddleware creates a Gin middleware that validates the access token
// and injects user_id into the Gin Context.
func (s *AuthService) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header is missing"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header must be Bearer format"})
			return
		}

		tokenString := parts[1]

		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}

		tokenType, _ := claims["type"].(string)
		if tokenType != "access" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token type"})
			return
		}

		userID, ok := claims["sub"].(string)
		if !ok || userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid user identifier in token"})
			return
		}

		// Inject userID into context
		c.Set("user_id", userID)
		c.Next()
	}
}
