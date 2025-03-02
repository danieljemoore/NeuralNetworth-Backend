// controllers/roundController.go
package controllers

import (
	"net/http"
	"strconv"
	"time"

	"midnight-trader/models"

	"github.com/gin-gonic/gin"
)

// RoundController handles HTTP requests for rounds.
type RoundController struct {
	RoundManager *RoundManagerWrapper
	Hub          *models.Hub
}

// NewRoundController returns a new RoundController instance.
func NewRoundController(rm *RoundManagerWrapper, hub *models.Hub) *RoundController {
	return &RoundController{
		RoundManager: rm,
		Hub:          hub,
	}
}

// StartRound initializes a new round.
func (rc *RoundController) StartRound(c *gin.Context) {
	rc.RoundManager.RoundLock.Lock()
	defer rc.RoundManager.RoundLock.Unlock()

	if rc.RoundManager.CurrentRound != nil && rc.RoundManager.CurrentRound.Status == "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A round is already active."})
		return
	}

	rc.RoundManager.StartNextRound()
	c.JSON(http.StatusOK, gin.H{
		"message": "Round started.",
		"round":   rc.RoundManager.CurrentRound,
	})
}

// EndRound manually ends the current round.
func (rc *RoundController) EndRound(c *gin.Context) {
	rc.RoundManager.RoundLock.Lock()
	defer rc.RoundManager.RoundLock.Unlock()

	if rc.RoundManager.CurrentRound == nil || rc.RoundManager.CurrentRound.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active round to end."})
		return
	}

	endedRound := rc.RoundManager.CurrentRound
	rc.RoundManager.EndRound()
	c.JSON(http.StatusOK, gin.H{
		"message": "Round ended.",
		"round":   endedRound,
	})
}

// EndRoundAutomatically is called (by timer) to end the round.
func (rc *RoundController) EndRoundAutomatically(roundID int) {
	rc.RoundManager.RoundLock.Lock()
	defer rc.RoundManager.RoundLock.Unlock()

	if rc.RoundManager.CurrentRound != nil &&
		rc.RoundManager.CurrentRound.Status == "active" &&
		rc.RoundManager.CurrentRound.ID == roundID {
		rc.RoundManager.EndRound()
	}
}

// GetRoundStatus returns the current round status.
func (rc *RoundController) GetRoundStatus(c *gin.Context) {
	rc.RoundManager.RoundLock.Lock()
	defer rc.RoundManager.RoundLock.Unlock()

	if rc.RoundManager.CurrentRound == nil {
		c.JSON(http.StatusOK, gin.H{"message": "No round in progress."})
		return
	}
	c.JSON(http.StatusOK, gin.H{"round": rc.RoundManager.CurrentRound})
}

// JoinRound allows a player to join an active round.
func (rc *RoundController) JoinRound(c *gin.Context) {
	player := c.Query("player")
	if player == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Player parameter is required."})
		return
	}

	rc.RoundManager.RoundLock.Lock()
	defer rc.RoundManager.RoundLock.Unlock()

	if rc.RoundManager.CurrentRound == nil || rc.RoundManager.CurrentRound.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active round to join."})
		return
	}

	// Check for duplicate join
	for _, p := range rc.RoundManager.CurrentRound.Participants {
		if p.Player == player {
			c.JSON(http.StatusOK, gin.H{
				"message": "Player already joined.",
				"round":   rc.RoundManager.CurrentRound,
			})
			return
		}
	}

	newParticipant := models.Portfolio{
		Player:    player,
		Companies: make(map[string]int),
		Funds:     1000.0,
	}
	rc.RoundManager.CurrentRound.Participants = append(rc.RoundManager.CurrentRound.Participants, newParticipant)

	// Broadcast that a player has joined.
	rc.Hub.Broadcast <- models.WSMessage{
		Event: "player_joined",
		Data: gin.H{
			"round_id": rc.RoundManager.CurrentRound.ID,
			"player":   newParticipant,
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Player joined the round.",
		"round":   rc.RoundManager.CurrentRound,
	})
}

// RoundTimer returns the elapsed and remaining time for the round.
func (rc *RoundController) RoundTimer(c *gin.Context) {
	rc.RoundManager.RoundLock.Lock()
	defer rc.RoundManager.RoundLock.Unlock()

	if rc.RoundManager.CurrentRound == nil || rc.RoundManager.CurrentRound.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active round."})
		return
	}

	elapsed := time.Since(rc.RoundManager.CurrentRound.StartTime)
	remaining := rc.RoundManager.RoundDuration - elapsed
	if remaining < 0 {
		remaining = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"elapsed":   elapsed.String(),
		"remaining": remaining.String(),
	})
}

// UpdatePortfolio processes portfolio adjustments (buy/sell/funds).
func (rc *RoundController) UpdatePortfolio(c *gin.Context) {
	player := c.Query("player")
	action := c.Query("action")    // "buy", "sell", "add_funds", "remove_funds"
	company := c.Query("company")  // for buy/sell
	sharesStr := c.Query("shares") // used for buy/sell
	amountStr := c.Query("amount") // used for funds

	if player == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Player parameter is required."})
		return
	}

	rc.RoundManager.RoundLock.Lock()
	defer rc.RoundManager.RoundLock.Unlock()

	if rc.RoundManager.CurrentRound == nil || rc.RoundManager.CurrentRound.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active round."})
		return
	}

	// Locate the participant.
	idx := -1
	for i, p := range rc.RoundManager.CurrentRound.Participants {
		if p.Player == player {
			idx = i
			break
		}
	}
	if idx == -1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Player not found in current round."})
		return
	}

	participant := &rc.RoundManager.CurrentRound.Participants[idx]
	switch action {
	case "buy":
		if company == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Company parameter is required for buying."})
			return
		}
		shares, err := strconv.Atoi(sharesStr)
		if err != nil || shares <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Valid shares parameter is required."})
			return
		}

		price := rc.RoundManager.GetCompanyPrice(company)
		totalCost := float64(shares) * price
		if participant.Funds < totalCost {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Insufficient funds."})
			return
		}

		participant.Funds -= totalCost
		participant.Companies[company] += shares

	case "sell":
		if company == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Company parameter is required for selling."})
			return
		}
		shares, err := strconv.Atoi(sharesStr)
		if err != nil || shares <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Valid shares parameter is required."})
			return
		}
		currentShares, exists := participant.Companies[company]
		if !exists || currentShares < shares {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Insufficient shares."})
			return
		}

		price := rc.RoundManager.GetCompanyPrice(company)
		totalGain := float64(shares) * price
		participant.Funds += totalGain
		participant.Companies[company] -= shares
		if participant.Companies[company] == 0 {
			delete(participant.Companies, company)
		}

	case "add_funds":
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil || amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Valid amount parameter is required."})
			return
		}
		participant.Funds += amount

	case "remove_funds":
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil || amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Valid amount parameter is required."})
			return
		}
		if participant.Funds < amount {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Insufficient funds."})
			return
		}
		participant.Funds -= amount

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action."})
		return
	}

	// Broadcast portfolio update.
	rc.Hub.Broadcast <- models.WSMessage{
		Event: "portfolio_updated",
		Data: gin.H{
			"round_id":  rc.RoundManager.CurrentRound.ID,
			"player":    participant.Player,
			"companies": participant.Companies,
			"funds":     participant.Funds,
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Portfolio updated.",
		"portfolio": participant,
	})
}
