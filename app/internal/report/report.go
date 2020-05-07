package report

import (
	"encoding/binary"
	"time"

	"app/internal/util"
)

const (
	// UploadKeyLen is the length, in bytes, of an UploadKey
	UploadKeyLen = 16

	pendingReportsCollection = "pending_reports"

	// 3 days - the period of during which a token is valid and may be verified.
	validityPeriod = time.Hour * 24 * 3
	// 4 days - the period of time - after the validity period has expired -
	// during which the token is still allocated in order to prevent
	// reallocation.
	allocationPeriod = time.Hour * 24 * 4
)

// An UploadKey is used to authorize the uploading of future reports.
type UploadKey [UploadKeyLen]byte

func genUploadKey() UploadKey {
	var k UploadKey
	util.ReadCryptoRandBytes(k[:])
	return k
}

// The layout of a pending report document in the database.
type pendingReportDoc struct {
	UploadKey UploadKey
	// The 9-bit key from the upload token, used to hedge against human mistakes
	// in transmitting or entering the upload token.
	TokenKey   uint16
	ReportData []byte
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
	now := time.Now()
	validityExp := now.Add(validityPeriod)
	allocationExp := validityExp.Add(allocationPeriod)
	doc := pendingReportDoc{
		UploadKey:            genUploadKey(),
		TokenKey:             0, // We'll explicitly set this below
		ReportData:           reportData,
		ValidityExpiration:   validityExp,
		AllocationExpiration: allocationExp,
	}

	pendingReports := ctx.FirestoreClient().Collection(pendingReportsCollection)

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
