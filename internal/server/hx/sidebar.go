package hx

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esnunes/prompter/internal/models"
)

type SidebarItem struct {
	ID         int64
	Title      string
	Status     string // "draft", "published"
	Processing bool   // true if repoStatus shows cloning/pulling/processing
	Unread     bool   // true if new assistant response since last_viewed_at
	RepoURL    string // shown only on dashboard
	UpdatedAt  time.Time
	Org        string // for URL construction
	Repo       string // for URL construction
}

type SidebarData struct {
	Items     []SidebarItem
	Scope     string // "all" (dashboard) or "repo"
	CurrentID int64  // highlighted item (0 if not on conversation page)
	RepoURL   string // repo URL for query param construction in template
}

// BuildSidebar creates sidebar data from a list of prompt requests, merging in
// processing state via getRepoStatus and computing unread flags.
func BuildSidebar(prs []models.PromptRequest, scope string, currentID int64, getRepoStatus func(int64) string) SidebarData {
	var items []SidebarItem
	for _, pr := range prs {
		// Parse org/repo from RepoURL (github.com/org/repo)
		parts := strings.SplitN(pr.RepoURL, "/", 3)
		var org, repoName string
		if len(parts) == 3 {
			org = parts[1]
			repoName = parts[2]
		}

		// Check processing state from in-memory status
		processing := false
		status := getRepoStatus(pr.ID)
		if status == "cloning" || status == "pulling" || status == "processing" {
			processing = true
		}

		// Compute unread: has assistant response newer than last_viewed_at
		unread := false
		if pr.LatestAssistantAt != nil && pr.ID != currentID {
			if pr.LastViewedAt == nil {
				unread = true
			} else if pr.LatestAssistantAt.After(*pr.LastViewedAt) {
				unread = true
			}
		}

		items = append(items, SidebarItem{
			ID:         pr.ID,
			Title:      pr.Title,
			Status:     pr.Status,
			Processing: processing,
			Unread:     unread,
			RepoURL:    pr.RepoURL,
			UpdatedAt:  pr.UpdatedAt,
			Org:        org,
			Repo:       repoName,
		})
	}

	// Sort: processing first, then drafts, then published; within each group by UpdatedAt DESC
	sortSidebarItems(items)

	// Extract repo URL for template query param construction
	var repoURL string
	if scope == "repo" && len(prs) > 0 {
		repoURL = prs[0].RepoURL
	}

	return SidebarData{
		Items:     items,
		Scope:     scope,
		CurrentID: currentID,
		RepoURL:   repoURL,
	}
}

// sortSidebarItems sorts items: processing first, then drafts, then published.
func sortSidebarItems(items []SidebarItem) {
	n := len(items)
	if n <= 1 {
		return
	}

	key := func(item SidebarItem) int {
		if item.Processing {
			return 0
		}
		if item.Status == "draft" {
			return 1
		}
		return 2
	}

	// Simple stable sort (bubble sort is fine for small N)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-1-i; j++ {
			if key(items[j]) > key(items[j+1]) {
				items[j], items[j+1] = items[j+1], items[j]
			}
		}
	}
}

func (h *Handler) handleSidebarFragment(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	repoURL := r.URL.Query().Get("repo_url")
	currentID, _ := strconv.ParseInt(r.URL.Query().Get("current_id"), 10, 64)

	var prs []models.PromptRequest
	var err error
	if scope == "repo" && repoURL != "" {
		prs, err = h.queries.ListPromptRequestsByRepoURL(repoURL, false)
	} else {
		prs, err = h.queries.ListPromptRequests(false)
	}
	if err != nil {
		log.Printf("sidebar query error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sidebar := BuildSidebar(prs, scope, currentID, h.getRepoStatus)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.sidebarTmpl.ExecuteTemplate(w, "sidebar.html", sidebar); err != nil {
		log.Printf("render error (sidebar): %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
