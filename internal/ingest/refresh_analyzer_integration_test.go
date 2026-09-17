//go:build integration

package ingest_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestRefreshAnalyzerHandlerFansOut(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	analyzer := tenant.Analyzers[0]
	provider := integration.Provider(analyzer.Provider)
	creds := integration.Credentials{Provider: provider, Subtype: analyzer.ProviderSubtype}
	src := newFakeAdapter(provider, time.Hour, model.ReadingKindLoadProfile, model.ReadingKindDaily, model.ReadingKindBilling)
	credID := uuid.New()

	run := func(mode string) []job.FetchReadingsPayload {
		enq := newRecordingEnqueuer()
		svc := ingestTestNewService(t, ingestTestNewRepos(pool), creds, src, enq, clock.NewFake(time.Now()), ingest.Options{})
		require.NoError(t, svc.RefreshAnalyzer(ctx, job.RefreshAnalyzerPayload{
			CompanyID: tenant.Company.ID, CredentialID: credID, AnalyzerID: analyzer.ID, Mode: mode,
		}))
		var out []job.FetchReadingsPayload
		for _, task := range enq.enqueued() {
			p, err := job.DecodeFetchReadings(task)
			require.NoError(t, err)
			out = append(out, p)
		}
		return out
	}
	hourly := run(job.RefreshModeHourly)
	require.Len(t, hourly, 1)
	require.Equal(t, model.ReadingKindLoadProfile, hourly[0].Kind)
	require.Nil(t, hourly[0].Window, "cursor-driven")
	require.Equal(t, credID, hourly[0].CredentialID)

	energy := run(job.RefreshModeEnergy)
	kinds := []model.ReadingKind{energy[0].Kind, energy[1].Kind}
	require.ElementsMatch(t, []model.ReadingKind{model.ReadingKindDaily, model.ReadingKindBilling}, kinds)

	other := testfixtures.NewTenant(t, ctx, pool, 2)
	enq := newRecordingEnqueuer()
	svc := ingestTestNewService(t, ingestTestNewRepos(pool), creds, src, enq, clock.NewFake(time.Now()), ingest.Options{})
	err := svc.RefreshAnalyzer(ctx, job.RefreshAnalyzerPayload{CompanyID: other.Company.ID, CredentialID: credID, AnalyzerID: analyzer.ID, Mode: "hourly"})
	require.Error(t, err, "another company's analyzer id resolves to nothing")
	require.Empty(t, enq.enqueued())
}
