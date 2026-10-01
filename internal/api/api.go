package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kaushalacts/vyom/internal/logging"
	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/internal/version"
	"github.com/kaushalacts/vyom/pkg/models"
	"github.com/kaushalacts/vyom/internal/providererr"
)

func RegisterRoutes(r chi.Router, log *logging.Logger, db repository.DB, cfg interface{}) {
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, req)
			log.Debug("HTTP request",
				"method", req.Method,
				"path", req.URL.Path,
				"duration", time.Since(start),
				"request_id", middleware.GetReqID(req.Context()),
			)
		})
	})

	r.Get("/api/v1/health", healthHandler)
	r.Get("/api/v1/version", versionHandler)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/assets", listAssetsHandler(db))
		r.Get("/assets/{id}", getAssetHandler(db))

		r.Get("/impacts", listImpactsHandler(db))
		r.Get("/impacts/top", topImpactsHandler(db))

		r.Get("/scans", listScansHandler(db))
		r.Get("/scans/{id}", getScanHandler(db))
		r.Get("/scans/latest", latestScanHandler(db))

		r.Get("/optimizations", listOptimizationsHandler(db))
		r.Get("/optimizations/{id}", getOptimizationHandler(db))
		r.Get("/optimizations/{id}/manifest", getManifestHandler(db))

		r.Get("/policies", listPoliciesHandler(db))
		r.Get("/policies/{name}", getPolicyHandler(db))

		r.Get("/providers", listProvidersHandler(db))
		r.Get("/providers/{id}", getProviderHandler(db))
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	info := version.Get()
	_ = json.NewEncoder(w).Encode(info)
}

func listAssetsHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := parsePageRequest(r)
		filter := parseAssetFilter(r)
		result, err := db.ListAssets(r.Context(), filter, page)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, result)
	}
}

func getAssetHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		asset, err := db.GetAsset(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, asset)
	}
}

func listImpactsHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := parsePageRequest(r)
		result, err := db.ListImpacts(r.Context(), page)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, result)
	}
}

func topImpactsHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 20
		if l := r.URL.Query().Get("limit"); l != "" {
			fmt.Sscanf(l, "%d", &limit)
		}
		impacts, err := db.TopImpacts(r.Context(), limit)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, impacts)
	}
}

func listScansHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := parsePageRequest(r)
		result, err := db.ListScans(r.Context(), page)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, result)
	}
}

func getScanHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		scan, err := db.GetScan(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, scan)
	}
}

func latestScanHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scan, err := db.LatestScan(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		if scan == nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, scan)
	}
}

func listOptimizationsHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := parsePageRequest(r)
		result, err := db.ListOptimizationJobs(r.Context(), page)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, result)
	}
}

func getOptimizationHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, err := db.GetOptimizationJob(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, job)
	}
}

func getManifestHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		manifest, err := db.GetManifest(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, manifest)
	}
}

func listPoliciesHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		policies, err := db.ListPolicies(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, policies)
	}
}

func getPolicyHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		policy, err := db.GetPolicy(r.Context(), name)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, policy)
	}
}

func listProvidersHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providers, err := db.ListProviders(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, providers)
	}
}

func getProviderHandler(db repository.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		provider, err := db.GetProvider(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, provider)
	}
}

func parsePageRequest(r *http.Request) models.PageRequest {
	limit := 50
	offset := 0
	fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
	fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &offset)
	sort := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")
	return models.PageRequest{Limit: limit, Offset: offset, Sort: sort, Order: models.SortOrder(order)}
}

func parseAssetFilter(r *http.Request) models.AssetFilter {
	return models.AssetFilter{
		ScanID:           r.URL.Query().Get("scan_id"),
		CDNProvider:      r.URL.Query().Get("cdn_provider"),
		StorageProvider:  r.URL.Query().Get("storage_provider"),
		Format:           r.URL.Query().Get("format"),
		Kind:             r.URL.Query().Get("kind"),
		Search:           r.URL.Query().Get("search"),
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		writeError(w, providererr.Wrap(err, "api", "writeJSON", providererr.KindInternal))
	}
}

func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	status := http.StatusInternalServerError

	var pe *providererr.Error
	if providererr.As(err, &pe) {
		switch pe.Kind {
		case providererr.KindNotFound:
			status = http.StatusNotFound
		case providererr.KindConfiguration, providererr.KindMalformedResponse:
			status = http.StatusBadRequest
		case providererr.KindAuthentication:
			status = http.StatusUnauthorized
		case providererr.KindAuthorization:
			status = http.StatusForbidden
		case providererr.KindRateLimited:
			status = http.StatusTooManyRequests
		case providererr.KindUnavailable:
			status = http.StatusServiceUnavailable
		}
	}

	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": providererr.Summarize(err),
	})
}