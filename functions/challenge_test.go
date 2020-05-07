package functions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"upload-token.functions/internal/pow"
	"upload-token.functions/internal/util"
)

func TestChallenge(t *testing.T) {
	firestore := util.NewTestFirestore(t)

	req, err := http.NewRequestWithContext(context.Background(), "GET", "/challenge", nil)
	assert.Nil(t, err)
	r := httptest.NewRecorder()
	ctx, err := util.NewTestRequestContext(r, req, firestore)
	assert.Nil(t, err)

	ChallengeHandler(ctx)

	// First, unmarshal using pow.Challenge in order to benefit from its
	// validation.
	var c0 pow.Challenge
	err = json.Unmarshal(r.Body.Bytes(), &c0)
	assert.Nil(t, err)

	// Second, unmarshal into a map so that we can inspect its contents.
	var c1 map[string]interface{}
	err = json.Unmarshal(r.Body.Bytes(), &c1)
	assert.Nil(t, err)
	assert.Equal(t, c1["work_factor"], float64(pow.DefaultWorkFactor))
}
