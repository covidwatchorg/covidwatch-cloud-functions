package functions

import (
	"encoding/json"
	"errors"
	"log"
	"os"

	"upload-token.functions/internal/pow"
	"upload-token.functions/internal/report"
	"upload-token.functions/internal/util"
)

// If the environment variable ALLOW_EMPTY_CHALLENGE_SOLUTION is set, then if an
// empty challenge solution is given, simply skip verification. This is useful
// in testing.
var allowEmptyChallengeSolution = false

func init() {
	if os.Getenv("ALLOW_EMPTY_CHALLENGE_SOLUTION") != "" {
		log.Println("Detected ALLOW_EMPTY_CHALLENGE_SOLUTION")
		allowEmptyChallengeSolution = true
	}
}

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

// ReportHandler is a handler for the /report endpoint.
var ReportHandler = util.MakeHTTPHandler(reportHandler)

func reportHandler(ctx *util.Context) util.StatusError {
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
		if !allowEmptyChallengeSolution || *req.Challenge != emptyChallgeSolution {
			if err := pow.ValidateSolution(ctx, req.Challenge); err != nil {
				return err
			}
		}

		token, key, err := report.StorePendingReport(ctx, req.Report.Data)
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

// ValidateHandler is a handler for the /validate endpoint.
var ValidateHandler = util.MakeHTTPHandler(validateHandler)

func validateHandler(ctx *util.Context) util.StatusError {
	if err := ctx.ValidateRequestMethod("POST", ""); err != nil {
		return err
	}

	var req validateRequest
	if err := json.NewDecoder(ctx.HTTPRequest().Body).Decode(&req); err != nil {
		return util.JSONToStatusError(err)
	}

	if err := report.ValidatePendingReport(ctx, req.UploadToken); err != nil {
		return err
	}

	return nil
}
