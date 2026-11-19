package store

import (
	"errors"
	"slices"
	"testing"
)

func TestWebhookAndSetWebhook(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")

	url, last, err := s.Webhook(ctx, u.ID)
	if err != nil || url != nil || last != nil {
		t.Errorf("Webhook(initial) = %s, %v, %v; want nil, nil, nil", str(url), last, err)
	}

	hook1 := "https://hooks.slack.com/services/T000/B000/SATU"
	if err := s.SetWebhook(ctx, u.ID, &hook1); err != nil {
		t.Fatalf("SetWebhook(hook1): %v", err)
	}
	if url, last, err = s.Webhook(ctx, u.ID); err != nil || str(url) != `"https://hooks.slack.com/services/T000/B000/SATU"` || last != nil {
		t.Errorf("Webhook(after set) = %s, %v, %v", str(url), last, err)
	}

	// Claim a digest, replace the URL, then turn the webhook off: webhook_terakhir stays recorded.
	today := tanggal(2026, 11, 19)
	if ok, err := s.ClaimDigest(ctx, u.ID, today); err != nil || !ok {
		t.Fatalf("ClaimDigest = %v, %v", ok, err)
	}
	hook2 := "https://discord.com/api/webhooks/123/abc"
	if err := s.SetWebhook(ctx, u.ID, &hook2); err != nil {
		t.Fatalf("SetWebhook(hook2): %v", err)
	}
	if url, last, err = s.Webhook(ctx, u.ID); err != nil || str(url) != `"https://discord.com/api/webhooks/123/abc"` || !sama(last, &today) {
		t.Errorf("Webhook(after replace) = %s, %v, %v", str(url), last, err)
	}
	if err := s.SetWebhook(ctx, u.ID, nil); err != nil {
		t.Fatalf("SetWebhook(nil): %v", err)
	}
	if url, last, err = s.Webhook(ctx, u.ID); err != nil || url != nil || !sama(last, &today) {
		t.Errorf("Webhook(after clear) = %s, %v, %v", str(url), last, err)
	}

	unknown := "00000000-0000-4000-8000-000000000999"
	if _, _, err := s.Webhook(ctx, unknown); !errors.Is(err, ErrNotFound) {
		t.Errorf("Webhook(unknown) err = %v, want ErrNotFound", err)
	}
	if err := s.SetWebhook(ctx, unknown, &hook1); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetWebhook(unknown) err = %v, want ErrNotFound", err)
	}
}

func TestDigestDueAndClaimDigest(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()

	yesterday := tanggal(2026, 11, 18)
	today := tanggal(2026, 11, 19)
	tomorrow := tanggal(2026, 11, 20)

	noHook := penggunaUji(t, s, "Tanpa Webhook")
	neverSent := penggunaUji(t, s, "Belum Pernah")
	sentYesterday := penggunaUji(t, s, "Kemarin")
	sentToday := penggunaUji(t, s, "Hari Ini")

	if err := s.SetWebhook(ctx, neverSent.ID, ptr("https://contoh.id/hook-1")); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWebhook(ctx, sentYesterday.ID, ptr("https://contoh.id/hook-2")); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimDigest(ctx, sentYesterday.ID, yesterday); err != nil || !ok {
		t.Fatalf("ClaimDigest(sentYesterday, yesterday) = %v, %v", ok, err)
	}
	if err := s.SetWebhook(ctx, sentToday.ID, ptr("https://contoh.id/hook-3")); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimDigest(ctx, sentToday.ID, today); err != nil || !ok {
		t.Fatalf("ClaimDigest(sentToday, today) = %v, %v", ok, err)
	}

	// Only neverSent and sentYesterday are due on today, ordered by user id.
	due, err := s.DigestDue(ctx, today)
	if err != nil {
		t.Fatalf("DigestDue: %v", err)
	}
	var gotIDs []string
	for _, u := range due {
		gotIDs = append(gotIDs, u.ID)
		if u.Nama == "" || u.WebhookURL == "" {
			t.Errorf("DigestDue row missing fields: %+v", u)
		}
	}
	wantIDs := []string{neverSent.ID, sentYesterday.ID}
	slices.Sort(wantIDs)
	if !slices.Equal(gotIDs, wantIDs) {
		t.Errorf("DigestDue(today) = %v, want %v", gotIDs, wantIDs)
	}

	// ClaimDigest claims once per day for a user with an active webhook.
	if ok, err := s.ClaimDigest(ctx, neverSent.ID, today); err != nil || !ok {
		t.Errorf("first ClaimDigest(neverSent, today) = %v, %v; want true, nil", ok, err)
	}
	if ok, err := s.ClaimDigest(ctx, neverSent.ID, today); err != nil || ok {
		t.Errorf("second ClaimDigest(neverSent, today) = %v, %v; want false, nil", ok, err)
	}
	if ok, err := s.ClaimDigest(ctx, neverSent.ID, yesterday); err != nil || ok {
		t.Errorf("ClaimDigest(neverSent, yesterday after today) = %v, %v; want false, nil", ok, err)
	}
	if _, last, err := s.Webhook(ctx, neverSent.ID); err != nil || !sama(last, &today) {
		t.Errorf("Webhook(neverSent) last = %v, %v; want %s", last, err, today.Format("2006-01-02"))
	}

	// Claiming the remaining due user empties DigestDue for today (still non-nil slice).
	if ok, err := s.ClaimDigest(ctx, sentYesterday.ID, today); err != nil || !ok {
		t.Errorf("ClaimDigest(sentYesterday, today) = %v, %v; want true, nil", ok, err)
	}
	if due, err = s.DigestDue(ctx, today); err != nil || due == nil || len(due) != 0 {
		t.Errorf("DigestDue(all claimed) = %v, %v; want empty non-nil slice", due, err)
	}

	// Tomorrow, all three users with webhooks can be claimed again.
	if due, err = s.DigestDue(ctx, tomorrow); err != nil || len(due) != 3 {
		t.Errorf("DigestDue(tomorrow) = %d rows, %v; want 3", len(due), err)
	}
	if ok, err := s.ClaimDigest(ctx, neverSent.ID, tomorrow); err != nil || !ok {
		t.Errorf("ClaimDigest(neverSent, tomorrow) = %v, %v; want true, nil", ok, err)
	}

	// Users without a webhook or unknown user IDs cannot claim a digest.
	if ok, err := s.ClaimDigest(ctx, noHook.ID, today); err != nil || ok {
		t.Errorf("ClaimDigest(noHook) = %v, %v; want false, nil", ok, err)
	}
	if ok, err := s.ClaimDigest(ctx, "00000000-0000-4000-8000-000000000999", today); err != nil || ok {
		t.Errorf("ClaimDigest(unknown) = %v, %v; want false, nil", ok, err)
	}
}
