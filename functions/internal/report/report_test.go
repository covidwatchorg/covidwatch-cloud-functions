package report

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"upload-token.functions/internal/util"
)

func TestReport(t *testing.T) {
	firestore := util.NewTestFirestore(t)

	ctx, err := util.NewTestContext(context.Background(), firestore)
	assert.Nil(t, err)

	//
	// Store a pending report
	//

	newReport := func() (UploadToken, UploadKey, func() pendingReportDoc) {
		const clientData = "hello, world"
		token, key, err := StorePendingReport(ctx, []byte(clientData))
		assert.Nil(t, err)

		getDoc := func() pendingReportDoc {
			snapshot, err := ctx.FirestoreClient().Collection(pendingReportsCollection).Doc(token.idString()).Get(ctx)
			assert.Nil(t, err)
			var doc pendingReportDoc
			assert.Nil(t, snapshot.DataTo(&doc))
			return doc
		}

		doc := getDoc()
		assert.Equal(t, doc.UploadKey, key)
		assert.Equal(t, doc.TokenKey, token.key())
		assert.Equal(t, doc.ReportData, []byte(clientData))
		assert.Equal(t, doc.Validated, false)

		return token, key, getDoc
	}

	token, key, getDoc := newReport()

	//
	// Test a number of validation attempts that should fail
	//

	// The wrong token ID
	err = ValidatePendingReport(ctx, newUploadToken(0, token.key()))
	assert.Equal(t, util.NotFoundError, err)
	// The wrong key
	err = ValidatePendingReport(ctx, newUploadToken(token.id(), 0))
	assert.Equal(t, util.NotFoundError, err)
	// Expired token
	ctx.Elapse(validityPeriod + 1)
	err = ValidatePendingReport(ctx, token)
	assert.Equal(t, util.NotFoundError, err)

	//
	// Test a validation that should succeed
	//

	// Generate a new report since the old one has expired, and so cannot be
	// validated successfully.
	token, key, getDoc = newReport()

	err = ValidatePendingReport(ctx, token)
	assert.Nil(t, err)

	doc := getDoc()
	assert.Equal(t, doc.UploadKey, key)
	assert.Equal(t, doc.TokenKey, token.key())
	assert.Equal(t, doc.ReportData, []byte{})
	assert.Equal(t, doc.Validated, true)

	//
	// Test that validating an already-validated token should fail
	//

	err = ValidatePendingReport(ctx, token)
	assert.Equal(t, util.NotFoundError, err)
}

func TestKeyJSON(t *testing.T) {
	for i := 0; i < 1024; i++ {
		k0 := genUploadKey()
		bytes, err := json.Marshal(k0)
		assert.Nil(t, err)
		var k1 UploadKey
		err = json.Unmarshal(bytes, &k1)
		assert.Nil(t, err)
		assert.Equal(t, k0, k1)
	}
}
