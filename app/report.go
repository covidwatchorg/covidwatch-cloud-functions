package app

import (
	"encoding/json"
	"errors"
	"net/http"

	"app/internal/pow"
	"app/internal/report"
	"app/internal/util"
)

// reportRequest is the body of a POST request to the /report endpoint.
type reportRequest struct {
	// Must have exactly one of Challenge or UploadKey.
	Challenge *pow.ChallengeSolution `json:"challenge"`
	UploadKey *report.UploadKey      `json:"upload_key"`
	Report    reportObj              `json:"report"`
}

type reportObj struct {
	Data []byte `json:"data"`
}

type reportResponse struct {
	UploadToken report.UploadToken `json:"upload_token"`
	UploadKey   report.UploadKey   `json:"upload_key"`
}

// ReportHandler is a handler for the /report endpoint.
func ReportHandler(w http.ResponseWriter, r *http.Request) {
	ctx, err := util.NewContext(w, r)
	if err != nil {
		return
	}

	if err = util.ValidateRequestMethod(&ctx, "POST", ""); err != nil {
		return
	}

	var req reportRequest
	if err := json.NewDecoder(ctx.HTTPRequest().Body).Decode(&req); err != nil {
		err := util.JSONToStatusError(err)
		ctx.WriteStatusError(err)
		return
	}

	switch {
	case req.Challenge != nil && req.UploadKey != nil:
		err := util.NewBadRequestError(errors.New("can only have proof of work challenge solution or upload key, not both"))
		ctx.WriteStatusError(err)
	case req.Challenge != nil:
		if err := pow.ValidateSolution(&ctx, req.Challenge); err != nil {
			ctx.WriteStatusError(err)
			return
		}

		token, key, err := report.StorePendingReport(&ctx, req.Report.Data)
		if err != nil {
			ctx.WriteStatusError(err)
			return
		}

		json.NewEncoder(w).Encode(reportResponse{
			UploadToken: token,
			UploadKey:   key,
		})
	case req.UploadKey != nil:
	default:
		err := util.NewBadRequestError(errors.New("missing proof of work challenge solution"))
		ctx.WriteStatusError(err)
	}
}
