package artifact

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
)

type LazadaSellerBackfillStats struct {
	Scanned         int
	Updated         int
	MissingArtifact int
	MissingSeller   int
	ReadErrors      int
}

func (s *Service) BackfillLazadaEmailSellerNames(
	billRepo *repository.BillRepo,
	auditRepo *repository.AuditLogRepo,
) (LazadaSellerBackfillStats, error) {
	stats := LazadaSellerBackfillStats{}
	if s == nil || billRepo == nil {
		return stats, nil
	}

	targets, err := billRepo.ListLazadaEmailSellerBackfillTargets()
	if err != nil {
		return stats, err
	}

	for _, target := range targets {
		stats.Scanned++
		sellerName, art, foundArtifact, err := s.lazadaSellerNameFromArtifacts(target)
		if err != nil {
			stats.ReadErrors++
			if s.logger != nil {
				s.logger.Warn("lazada seller backfill: artifact read failed",
					zap.String("bill_id", target.ID),
					zap.String("order_id", target.OrderID),
					zap.Error(err),
				)
			}
			continue
		}
		if !foundArtifact {
			stats.MissingArtifact++
			continue
		}
		if sellerName == "" {
			stats.MissingSeller++
			continue
		}
		if strings.TrimSpace(target.SellerName) == sellerName {
			continue
		}

		updated, oldSeller, err := billRepo.UpdateLazadaEmailSellerName(target.ID, sellerName)
		if err != nil {
			return stats, fmt.Errorf("update lazada email seller name: %w", err)
		}
		if !updated {
			continue
		}
		stats.Updated++
		if auditRepo != nil {
			billID := target.ID
			_ = auditRepo.Log(models.AuditEntry{
				Action:   "lazada_email_seller_backfilled",
				TargetID: &billID,
				Source:   "lazada_email",
				Level:    "info",
				Detail: map[string]interface{}{
					"order_id":      target.OrderID,
					"artifact_id":   art.ID,
					"old_seller":    oldSeller,
					"new_seller":    sellerName,
					"email_message": target.EmailMessageID,
				},
			})
		}
	}
	return stats, nil
}

func (s *Service) lazadaSellerNameFromArtifacts(target repository.LazadaSellerBackfillTarget) (string, models.BillArtifact, bool, error) {
	artifacts, err := s.repo.ListByBill(target.ID)
	if err != nil {
		return "", models.BillArtifact{}, false, err
	}
	candidates := lazadaSellerCandidateArtifacts(artifacts, target.EmailMessageID)
	if len(candidates) == 0 {
		return "", models.BillArtifact{}, false, nil
	}

	var firstErr error
	for _, candidate := range candidates {
		data, art, err := s.Read(candidate.ID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if art == nil {
			continue
		}
		var sellerName string
		switch art.Kind {
		case "email_html":
			sellerName = repository.ExtractLazadaSellerName("", string(data))
		case "email_text":
			sellerName = repository.ExtractLazadaSellerName(string(data), "")
		default:
			continue
		}
		if sellerName != "" {
			return sellerName, *art, true, nil
		}
	}
	if firstErr != nil {
		return "", models.BillArtifact{}, true, firstErr
	}
	return "", models.BillArtifact{}, true, nil
}

func lazadaSellerCandidateArtifacts(artifacts []models.BillArtifact, messageID string) []models.BillArtifact {
	isBodyArtifact := func(a models.BillArtifact) bool {
		return a.Kind == "email_html" || a.Kind == "email_text"
	}
	messageID = strings.TrimSpace(messageID)
	candidates := []models.BillArtifact{}
	seen := map[string]bool{}

	if messageID != "" {
		for _, art := range artifacts {
			if !isBodyArtifact(art) || lazadaArtifactMessageID(art) != messageID {
				continue
			}
			candidates = append(candidates, art)
			seen[art.ID] = true
		}
	}
	for _, art := range artifacts {
		if !isBodyArtifact(art) || seen[art.ID] {
			continue
		}
		candidates = append(candidates, art)
	}
	return candidates
}

func lazadaArtifactMessageID(art models.BillArtifact) string {
	if len(art.SourceMeta) == 0 {
		return ""
	}
	var meta struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(art.SourceMeta, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.MessageID)
}
