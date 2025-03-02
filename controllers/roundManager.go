// controllers/roundManager.go
package controllers

import (
	"log"
	"sort"
	"time"

	"context"
	"midnight-trader/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// CompanyCollection should be initialized elsewhere.

// RoundManagerWrapper is a local wrapper around models.RoundManager
// which enables us to define new methods.
type RoundManagerWrapper struct {
	models.RoundManager
}

// NewRoundManager creates a new RoundManagerWrapper instance.
func NewRoundManager(hub *models.Hub, duration time.Duration, totalRounds int) *RoundManagerWrapper {
	return &RoundManagerWrapper{
		models.RoundManager{
			Hub:           hub,
			RoundDuration: duration,
			TotalRounds:   totalRounds,
			TimerStopChan: make(chan struct{}),
		},
	}
}

// Start initiates the round sequence.
func (rm *RoundManagerWrapper) Start() {
	rm.StartNextRound()
}

// StartNextRound initiates the next round if conditions are met.
func (rm *RoundManagerWrapper) StartNextRound() {
	rm.RoundLock.Lock()
	defer rm.RoundLock.Unlock()

	if rm.TotalRounds > 0 && rm.CompletedRounds >= rm.TotalRounds {
		log.Println("All rounds completed.")
		rm.Hub.Broadcast <- models.WSMessage{
			Event: "all_rounds_completed",
			Data:  nil,
		}
		return
	}

	if rm.CurrentRound != nil && rm.CurrentRound.Status == "active" {
		log.Println("A round is already active.")
		return
	}

	rm.CurrentRound = &models.RoundState{
		ID:           rm.generateRoundID(),
		Status:       "active",
		StartTime:    time.Now(),
		Participants: []models.Portfolio{},
	}

	// Append historical data if necessary.
	if err := rm.AppendGeneratedHistoricalData(); err != nil {
		log.Println("Failed to append historical data:", err)
	}

	rm.Hub.Broadcast <- models.WSMessage{
		Event: "round_started",
		Data: gin.H{
			"round_id":   rm.CurrentRound.ID,
			"start_time": rm.CurrentRound.StartTime.UTC().String(),
		},
	}

	// Set up the auto-end timer.
	rm.Timer = time.AfterFunc(rm.RoundDuration, func() {
		rm.EndRoundAutomatically()
	})

	// Start ticker for periodic timer updates.
	rm.TimerTicker = time.NewTicker(1 * time.Second)
	go rm.sendTimerUpdates()

	log.Printf("Round %d started.", rm.CurrentRound.ID)
}

// AppendGeneratedHistoricalData is a placeholder; replace with your logic if needed.
func (rm *RoundManagerWrapper) AppendGeneratedHistoricalData() error {
	return nil
}

// EndRound manually ends the current round.
func (rm *RoundManagerWrapper) EndRound() {
	rm.RoundLock.Lock()
	if rm.CurrentRound == nil || rm.CurrentRound.Status != "active" {
		rm.RoundLock.Unlock()
		log.Println("No active round to end.")
		return
	}

	// Stop timers.
	if rm.Timer != nil {
		rm.Timer.Stop()
		rm.Timer = nil
	}
	if rm.TimerTicker != nil {
		rm.TimerTicker.Stop()
		rm.TimerTicker = nil
	}

	// Debug: log each participant's portfolio value.
	for i, p := range rm.CurrentRound.Participants {
		value := rm.calculatePortfolioValue(p)
		log.Printf("Participant %d (%s) portfolio value: %f", i, p.Player, value)
	}

	// Determine the winner.
	winner := rm.determineWinner()
	if winner != nil {
		rm.CurrentRound.Winner = winner
	} else {
		log.Println("No winner could be determined.")
	}

	rm.CurrentRound.Status = "ended"
	rm.CurrentRound.EndTime = time.Now()

	// Compute leaderboard and broadcast leaderboard update.
	leaderboard := rm.GetLeaderboard()

	rm.Hub.Broadcast <- models.WSMessage{
		Event: "round_ended",
		Data: gin.H{
			"round_id":    rm.CurrentRound.ID,
			"end_time":    rm.CurrentRound.EndTime.UTC().String(),
			"winner":      rm.CurrentRound.Winner,
			"leaderboard": leaderboard,
		},
	}

	rm.CompletedRounds++
	rm.CurrentRound = nil
	rm.RoundLock.Unlock()

	// Trigger the next round.
	rm.StartNextRound()
}

// EndRoundAutomatically ends the round when the timer expires.
func (rm *RoundManagerWrapper) EndRoundAutomatically() {
	log.Println("Auto-ending the current round.")
	rm.EndRound()
}

// sendTimerUpdates sends periodic timer updates to clients.
func (rm *RoundManagerWrapper) sendTimerUpdates() {
	for {
		select {
		case <-rm.TimerTicker.C:
			rm.RoundLock.Lock()
			if rm.CurrentRound == nil || rm.CurrentRound.Status != "active" {
				rm.RoundLock.Unlock()
				return
			}
			elapsed := time.Since(rm.CurrentRound.StartTime)
			remaining := rm.RoundDuration - elapsed
			if remaining < 0 {
				remaining = 0
			}
			currentRoundID := rm.CurrentRound.ID
			rm.RoundLock.Unlock()

			rm.Hub.Broadcast <- models.WSMessage{
				Event: "timer_update",
				Data: gin.H{
					"round_id":  currentRoundID,
					"elapsed":   elapsed.String(),
					"remaining": remaining.String(),
				},
			}
		case <-rm.TimerStopChan:
			return
		}
	}
}

// generateRoundID returns a unique round ID.
func (rm *RoundManagerWrapper) generateRoundID() int {
	return int(time.Now().Unix())
}

// determineWinner returns the participant with the highest portfolio value.
func (rm *RoundManagerWrapper) determineWinner() *models.Portfolio {
	if rm.CurrentRound == nil || len(rm.CurrentRound.Participants) == 0 {
		return nil
	}

	var winner *models.Portfolio
	highestValue := -1.0

	for i := range rm.CurrentRound.Participants {
		participant := &rm.CurrentRound.Participants[i]
		totalValue := rm.calculatePortfolioValue(*participant)
		if totalValue > highestValue {
			highestValue = totalValue
			winner = participant
		}
	}

	return winner
}

// calculatePortfolioValue calculates the total value of a participant's portfolio.
func (rm *RoundManagerWrapper) calculatePortfolioValue(p models.Portfolio) float64 {
	total := p.Funds
	for ticker, shares := range p.Companies {
		price := rm.GetCompanyPrice(ticker)
		total += float64(shares) * price
	}
	return total
}

// GetCompanyPrice retrieves the current price of a company by its ticker.
func (rm *RoundManagerWrapper) GetCompanyPrice(ticker string) float64 {
	company, err := rm.GetCompanyByTicker(ticker)
	if err != nil {
		log.Printf("GetCompanyPrice error: %v", err)
		return 0.0
	}
	if company == nil {
		log.Printf("GetCompanyPrice: company with ticker %s not found", ticker)
		return 0.0
	}
	return company.StockPrice
}

// GetCompanyByTicker retrieves a company by ticker from MongoDB.
func (rm *RoundManagerWrapper) GetCompanyByTicker(ticker string) (*models.Company, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"ticker": ticker}
	var company models.Company
	err := CompanyCollection.FindOne(ctx, filter).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &company, nil
}

// GetCurrentRound provides thread-safe access to the current round.
func (rm *RoundManagerWrapper) GetCurrentRound() *models.RoundState {
	rm.RoundLock.Lock()
	defer rm.RoundLock.Unlock()
	return rm.CurrentRound
}

// GetLeaderboard returns a sorted slice of portfolios with the highest portfolio first.
func (rm *RoundManagerWrapper) GetLeaderboard() []models.Portfolio {
	// Ensure thread-safe access if this function is called outside
	// an existing Lock, otherwise consider removing additional locking.
	rm.RoundLock.Lock()
	defer rm.RoundLock.Unlock()

	// Make a copy of the participants to avoid modifying the original slice.
	leaderboard := make([]models.Portfolio, len(rm.CurrentRound.Participants))
	copy(leaderboard, rm.CurrentRound.Participants)

	sort.Slice(leaderboard, func(i, j int) bool {
		return rm.calculatePortfolioValue(leaderboard[i]) > rm.calculatePortfolioValue(leaderboard[j])
	})
	return leaderboard
}

// Global instance for accessing the RoundManagerWrapper from anywhere.
var CurrentRoundManager *RoundManagerWrapper

// GetCurrentRound returns the current round from the active round manager.
func GetCurrentRound() *models.RoundState {
	if CurrentRoundManager == nil {
		return nil
	}
	return CurrentRoundManager.GetCurrentRound()
}
