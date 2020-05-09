package functions

import (
	"encoding/json"

	"upload-token.functions/internal/pow"
	"upload-token.functions/internal/util"
)

// ChallengeHTTPHandler is an HTTP handler for the /challenge endpoint. It is
// intended to be registered as a Google Cloud Function by using the
// --entry-point flag to the `gcloud functions deploy` command.
var ChallengeHTTPHandler = util.MakeHTTPHandler(ChallengeHandler)

// ChallengeHandler is a handler for the /challenge endpoint.
func ChallengeHandler(ctx *util.Context) util.StatusError {
	if err := ctx.ValidateRequestMethod("GET", ""); err != nil {
		return err
	}

	c, err := pow.GenerateChallenge(ctx)
	if err != nil {
		return util.NewInternalServerError(err)
	}
	json.NewEncoder(ctx.HTTPResponseWriter()).Encode(c)

	return nil
}
