package controllers

import (
	"midnight-trader/models"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	currentRound        *models.RoundState
	currentParticipants []string
	roundLock           sync.Mutex
	roundDuration       = 2 * time.Minute // example duration; adjust as needed
)

func StartRound(c *gin.Context) {
	roundLock.Lock()
	defer roundLock.Unlock()

	if currentRound != nil && currentRound.Status == "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "round already active"})
		return
	}

	currentRound = &models.RoundState{
		ID:        1, // consider auto-incrementing or uuid
		Status:    "active",
		StartTime: time.Now(),
	}
	currentParticipants = []string{}

	// send round started update
	go SendRoundUpdate("round_started", map[string]interface{}{
		"round_id":   currentRound.ID,
		"start_time": currentRound.StartTime.UTC().String(),
	})

	// auto-end round after duration
	go func(roundID int) {
		time.Sleep(roundDuration)
		roundLock.Lock()
		defer roundLock.Unlock()
		if currentRound != nil && currentRound.Status == "active" && currentRound.ID == roundID {
			currentRound.Status = "ended"
			currentRound.EndTime = time.Now()
			SendRoundUpdate("round_ended", map[string]interface{}{
				"round_id": currentRound.ID,
				"end_time": currentRound.EndTime.UTC().String(),
			})
		}
	}(currentRound.ID)

	c.JSON(http.StatusOK, gin.H{"message": "round started", "round": currentRound, "participants": currentParticipants})
}
func StartRoundHandler(c *gin.Context) {
	roundLock.Lock()
	defer roundLock.Unlock()

	if currentRound != nil && currentRound.Status == "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "round already active"})
		return
	}

	currentRound = &models.RoundState{
		ID:        1, // consider auto-incrementing or uuid
		Status:    "active",
		StartTime: time.Now(),
	}
	currentParticipants = []string{}

	// send round started update
	go SendRoundUpdate("round_started", map[string]interface{}{
		"round_id":   currentRound.ID,
		"start_time": currentRound.StartTime.UTC().String(),
	})

	// auto-end round after duration
	go func(roundID int) {
		time.Sleep(roundDuration)
		roundLock.Lock()
		defer roundLock.Unlock()
		if currentRound != nil && currentRound.Status == "active" && currentRound.ID == roundID {
			currentRound.Status = "ended"
			currentRound.EndTime = time.Now()
			SendRoundUpdate("round_ended", map[string]interface{}{
				"round_id": currentRound.ID,
				"end_time": currentRound.EndTime.UTC().String(),
			})
		}
	}(currentRound.ID)

	c.JSON(http.StatusOK, gin.H{"message": "round started", "round": currentRound, "participants": currentParticipants})
}

func EndRound(c *gin.Context) {
	roundLock.Lock()
	defer roundLock.Unlock()

	if currentRound == nil || currentRound.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active round to end"})
		return
	}

	currentRound.Status = "ended"
	currentRound.EndTime = time.Now()
	SendRoundUpdate("round_ended", map[string]interface{}{
		"round_id": currentRound.ID,
		"end_time": currentRound.EndTime.UTC().String(),
	})
	c.JSON(http.StatusOK, gin.H{"message": "round ended", "round": currentRound, "participants": currentParticipants})
}

func EndRoundHandler(c *gin.Context) {
	roundLock.Lock()
	defer roundLock.Unlock()

	if currentRound == nil || currentRound.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active round to end"})
		return
	}

	currentRound.Status = "ended"
	currentRound.EndTime = time.Now()
	SendRoundUpdate("round_ended", map[string]interface{}{
		"round_id": currentRound.ID,
		"end_time": currentRound.EndTime.UTC().String(),
	})
	c.JSON(http.StatusOK, gin.H{"message": "round ended", "round": currentRound, "participants": currentParticipants})
}

func GetRoundStatus(c *gin.Context) {
	roundLock.Lock()
	defer roundLock.Unlock()

	if currentRound == nil {
		c.JSON(http.StatusOK, gin.H{"message": "no round in progress"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"round": currentRound, "participants": currentParticipants})
}

func GetRoundStatusHandler(c *gin.Context) {
	roundLock.Lock()
	defer roundLock.Unlock()

	if currentRound == nil {
		c.JSON(http.StatusOK, gin.H{"message": "no round in progress"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"round": currentRound, "participants": currentParticipants})
}

func JoinRound(c *gin.Context) {
	player := c.Query("player")
	if player == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "player parameter is required"})
		return
	}

	roundLock.Lock()
	defer roundLock.Unlock()

	if currentRound == nil || currentRound.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active round to join"})
		return
	}

	for _, p := range currentParticipants {
		if p == player {
			c.JSON(http.StatusOK, gin.H{"message": "already joined", "round": currentRound, "participants": currentParticipants})
			return
		}
	}

	currentParticipants = append(currentParticipants, player)
	SendRoundUpdate("player_joined", map[string]interface{}{
		"round_id": currentRound.ID,
		"player":   player,
	})
	c.JSON(http.StatusOK, gin.H{"message": "joined round", "round": currentRound, "participants": currentParticipants})
}

func JoinRoundHandler(c *gin.Context) {
	player := c.Query("player")
	if player == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "player parameter is required"})
		return
	}

	roundLock.Lock()
	defer roundLock.Unlock()

	if currentRound == nil || currentRound.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active round to join"})
		return
	}

	for _, p := range currentParticipants {
		if p == player {
			c.JSON(http.StatusOK, gin.H{"message": "already joined", "round": currentRound, "participants": currentParticipants})
			return
		}
	}

	currentParticipants = append(currentParticipants, player)
	SendRoundUpdate("player_joined", map[string]interface{}{
		"round_id": currentRound.ID,
		"player":   player,
	})
	c.JSON(http.StatusOK, gin.H{"message": "joined round", "round": currentRound, "participants": currentParticipants})
}
