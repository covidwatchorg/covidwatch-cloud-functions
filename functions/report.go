package functions

import (
	"encoding/json"
	"errors"
	"log"

	"upload-token.functions/internal/pow"
	"upload-token.functions/internal/report"
	"upload-token.functions/internal/util"
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

// Validate validates that r has exactly one of Challenge or UploadKey set (not
// both, and not neither).
func (r *reportRequest) Validate() util.StatusError {
	switch {
	case r.Challenge != nil && r.UploadKey != nil:
		return util.NewBadRequestError(errors.New("can only have proof of work challenge solution or upload key, not both"))
	case r.Challenge == nil && r.UploadKey == nil:
		return util.NewBadRequestError(errors.New("missing proof of work challenge solution or upload key"))
	default:
		return nil
	}
}

// ReportHTTPHandler is an HTTP handler for the /report endpoint. It is intended
// to be registered as a Google Cloud Function by using the --entry-point flag
// to the `gcloud functions deploy` command.
var ReportHTTPHandler = util.MakeHTTPHandler(ReportHandler)

// ReportHandler is a handler for the /report endpoint.
func ReportHandler(ctx *util.RequestContext) util.StatusError {
	if err := ctx.ValidateRequestMethod("POST", ""); err != nil {
		return err
	}

	var req reportRequest
	if err := json.NewDecoder(ctx.HTTPRequest().Body).Decode(&req); err != nil {
		return util.JSONToStatusError(err)
	}
	if err := req.Validate(); err != nil {
		return err
	}

	if req.Challenge != nil {
		var emptyChallgeSolution pow.ChallengeSolution
		if ctx.AllowEmptyChallengeSolution() && *req.Challenge == emptyChallgeSolution {
			log.Println("WARNING: Skipping challenge solution validation; ALLOW_EMPTY_CHALLENGE_SOLUTION is set and an empty challenge was supplied")
		} else {
			if err := pow.ValidateSolution(ctx.Inner(), req.Challenge); err != nil {
				return err
			}
		}

		token, key, err := report.StorePendingReport(ctx.Inner(), req.Report.Data)
		if err != nil {
			return err
		}

		json.NewEncoder(ctx.HTTPResponseWriter()).Encode(reportResponse{
			UploadToken: token,
			UploadKey:   key,
		})
	} else {
		// TODO(joshlf): Implement this case.
		return util.NewNotImplementedError()
	}

	return nil
}

type validateRequest struct {
	UploadToken report.UploadToken `json:"upload_token"`
}

// ValidateHTTPHandler is an HTTP handler for the /validate endpoint. It is
// intended to be registered as a Google Cloud Function by using the
// --entry-point flag to the `gcloud functions deploy` command.
var ValidateHTTPHandler = util.MakeHTTPHandler(ValidateHandler)

// ValidateHandler is a handler for the /validate endpoint.
func ValidateHandler(ctx *util.RequestContext) util.StatusError {
	if err := ctx.ValidateRequestMethod("POST", ""); err != nil {
		return err
	}

	var req validateRequest
	if err := json.NewDecoder(ctx.HTTPRequest().Body).Decode(&req); err != nil {
		return util.JSONToStatusError(err)
	}

	if err := report.ValidatePendingReport(ctx.Inner(), req.UploadToken); err != nil {
		return err
	}

	return nil
}
