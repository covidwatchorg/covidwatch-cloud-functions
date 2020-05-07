package report

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"time"

	"cloud.google.com/go/firestore"

	"upload-token.functions/internal/util"
)

const (
	// UploadKeyLen is the length, in bytes, of an UploadKey
	UploadKeyLen = 16

	pendingReportsCollection = "pending_reports"

	// 3 days - the period of time during which a token is valid and may be
	// verified.
	validityPeriod = time.Hour * 24 * 3
	// 4 days - the period of time after the validity period has expired during
	// which the token is still allocated in order to prevent reallocation.
	allocationPeriod = time.Hour * 24 * 4
)

// An UploadKey is used to authorize the uploading of future reports.
type UploadKey [UploadKeyLen]byte

func genUploadKey() UploadKey {
	var k UploadKey
	util.ReadCryptoRandBytes(k[:])
	return k
}

// MarshalJSON implements json.Marshaler.
func (k UploadKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(k[:])
}

var invalidUploadKeyError = util.NewBadRequestError(errors.New("invalid upload key"))

// UnmarshalJSON implements json.Unmarshaler.
func (k *UploadKey) UnmarshalJSON(b []byte) error {
	var bytes []byte
	err := json.Unmarshal(b, &bytes)
	if err != nil {
		return err
	}
	if len(bytes) != UploadKeyLen {
		return invalidUploadKeyError
	}
	copy(k[:], bytes)
	return nil
}

// The layout of a pending report document in the database.
type pendingReportDoc struct {
	UploadKey UploadKey
	// The 9-bit key from the upload token, used to hedge against human mistakes
	// in transmitting or entering the upload token.
	TokenKey uint16
	// The data of the report. Once this report has been logically removed but
	// is still in the database for allocation reasons (see comments on fields
	// below), this is zeroed to save space.
	ReportData []byte
	// Whether this report has already been validated. When a report is
	// validated, its token is logically removed from the database. However, in
	// order to prevent token reallocation, we leave the document in the
	// database with this flag set to true. See the comment on
	// ValidityExpiration for an explanation of why we want to prevent
	// reallocation.
	Validated bool
	// The time at which this document's upload token may no longer be
	// validated. Note that after the validity has expired, the document is
	// still not removed from the database until the allocation expiration has
	// been reached. This is in order to prevent the following scenario, which
	// would be possible if the two expirations were the same duration:
	// 1. User uploads a report and gets an upload token.
	// 2. User knows that they have a particular amount of time before their
	//    token will expire, so they wait until the last minute to call their
	//    health authority.
	// 3. Token expires, and document is remvoed from the database.
	// 4. A different user uploads a report, and is allocated the same token.
	// 5. Original user calls health authority, and provides token.
	// 6. Health authority verifies the original user's diagnosis, and verifies
	//    their token.
	// 7. The new report - associated with a different user - is published.
	//
	// We expect that a user will not wait /that/ long after they know that
	// their token has expired to try verifying their report with the health
	// authority, and so adding a period of time after which the token can no
	// longer be verified but during which the token cannot be re-allocated for
	// a new report minimizes the likelihood of this mistake happening.
	ValidityExpiration time.Time
	// The time at which this document is removed from the database, and the
	// token becomes available for allocation again.
	AllocationExpiration time.Time
}

// StorePendingReport stores the given report in the database as pending. It
// allocates a new upload token and upload key, and returns them.
func StorePendingReport(ctx *util.Context, reportData []byte) (UploadToken, UploadKey, util.StatusError) {
	return storePendingReport(ctx, ctx.FirestoreClient(), reportData)
}

func storePendingReport(ctx context.Context, client *firestore.Client, reportData []byte) (UploadToken, UploadKey, util.StatusError) {
	now := util.Now(ctx)
	validityExp := now.Add(validityPeriod)
	allocationExp := validityExp.Add(allocationPeriod)
	doc := pendingReportDoc{
		UploadKey:            genUploadKey(),
		TokenKey:             0, // We'll explicitly set this below
		Validated:            false,
		ReportData:           reportData,
		ValidityExpiration:   validityExp,
		AllocationExpiration: allocationExp,
	}

	pendingReports := client.Collection(pendingReportsCollection)

	// TODO(28): Implement a token allocation algorithm which guarantees that
	// the numerically smallest unallocated token is always chosen.
	//
	// For the time being, we simply generate a random token and hope for the
	// best. With 55 bits of entropy, it's very unlikely that this will ever be
	// a problem during testing before we implement the final algorithm.
	var bytes [8]byte
	util.ReadCryptoRandBytes(bytes[:])
	t := UploadToken{token: binary.BigEndian.Uint64(bytes[:])}
	doc.TokenKey = t.key()
	_, err := pendingReports.Doc(t.idString()).Create(ctx, doc)
	if err != nil {
		return UploadToken{}, UploadKey{}, util.NewInternalServerError(err)
	}
	return t, doc.UploadKey, nil
}

// ValidatePendingReport validates the pending report with the given token. On
// success, it performs the following operations under a single database
// transaction:
//  - Marks the token as validated
//  - Adds the upload key to the database of upload keys
//  - Adds the report to the database of published reports
//
// ValidatePendingReport returns a "not found" error under the following
// conditions:
//  - The token's ID doesn't identify a document in the database
//  - The token's key doesn't match the key stored in the document with the
//    token's ID
//  - The token's validity period has expired
//  - The token has already been validated
func ValidatePendingReport(ctx *util.Context, token UploadToken) util.StatusError {
	return validatePendingReport(ctx, ctx.FirestoreClient(), token)
}

func validatePendingReport(ctx context.Context, c *firestore.Client, token UploadToken) util.StatusError {
	// We perform the following steps under a transaction:
	// Reads:
	// - Get the document from the database
	// - Validate the token key
	// - Validate that the token has not already been validated
	// - Validate that the token has not expired
	// Writes:
	// - Mark the token as validated
	// - Add the upload key to the database of upload keys
	// - Add the report to the database of published reports
	return util.RunTransaction(ctx, c, func(ctx context.Context, txn *firestore.Transaction) util.StatusError {
		pendingReports := c.Collection(pendingReportsCollection)

		docID := token.idString()
		snapshot, err := txn.Get(pendingReports.Doc(docID))
		if err != nil {
			return util.FirestoreToStatusError(err)
		}
		var doc pendingReportDoc
		if err = snapshot.DataTo(&doc); err != nil {
			return util.FirestoreToStatusError(err)
		}

		if token.key() != doc.TokenKey || doc.Validated || util.Now(ctx).After(doc.ValidityExpiration) {
			// If the key doesn't match, then the token was entered incorrectly,
			// and so we treat this as a failed lookup.
			//
			// When a report is validated or expires, it is logically removed
			// from the database. Its document is only kept in order to prevent
			// the same token from being reallocated before the allocation
			// period has expired.
			return util.NotFoundError
		}

		// TODO(joshlf):
		// - Add the report to the database of published reports
		// - Add the upload key to the database of upload keys

		// So long as we're overwriting entire document anyway, clear the report
		// data to save space.
		doc.ReportData = nil
		doc.Validated = true
		if err = txn.Set(pendingReports.Doc(docID), doc); err != nil {
			return util.FirestoreToStatusError(err)
		}

		return nil
	})
}
