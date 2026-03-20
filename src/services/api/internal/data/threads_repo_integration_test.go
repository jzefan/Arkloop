//go:build !desktop

package data

import (
	"context"
	"testing"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"

	"github.com/google/uuid"
)

func setupThreadsTestRepos(t *testing.T) (*ThreadRepository, *MessageRepository, *AccountRepository, *UserRepository, *ProjectRepository, context.Context) {
	t.Helper()

	db := testutil.SetupPostgresDatabase(t, "api_go_threads_repo")
	ctx := context.Background()

	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	appDB, err := NewPool(ctx, db.DSN, PoolLimits{MaxConns: 32, MinConns: 0})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(func() { appDB.Close() })

	threadRepo, err := NewThreadRepository(appDB)
	if err != nil {
		t.Fatalf("new thread repo: %v", err)
	}
	messageRepo, err := NewMessageRepository(appDB)
	if err != nil {
		t.Fatalf("new message repo: %v", err)
	}
	accountRepo, err := NewAccountRepository(appDB)
	if err != nil {
		t.Fatalf("new account repo: %v", err)
	}
	userRepo, err := NewUserRepository(appDB)
	if err != nil {
		t.Fatalf("new user repo: %v", err)
	}
	projectRepo, err := NewProjectRepository(appDB)
	if err != nil {
		t.Fatalf("new project repo: %v", err)
	}

	return threadRepo, messageRepo, accountRepo, userRepo, projectRepo, ctx
}

func TestThreadRepositoryListSearchFork(t *testing.T) {
	threadRepo, messageRepo, accountRepo, userRepo, projectRepo, ctx := setupThreadsTestRepos(t)

	account, err := accountRepo.Create(ctx, "threads-list", "Threads List Account", "personal")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	user, err := userRepo.Create(ctx, "threads-list-user", "threads-list@test.com", "en")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	project, err := projectRepo.CreateDefaultForOwner(ctx, account.ID, user.ID)
	if err != nil {
		t.Fatalf("create default project: %v", err)
	}

	titleA := "thread-alpha"
	threadA, err := threadRepo.Create(ctx, account.ID, &user.ID, project.ID, &titleA, false)
	if err != nil {
		t.Fatalf("create thread a: %v", err)
	}

	titleB := "thread-beta"
	threadB, err := threadRepo.Create(ctx, account.ID, &user.ID, project.ID, &titleB, false)
	if err != nil {
		t.Fatalf("create thread b: %v", err)
	}

	listed, err := threadRepo.ListByOwner(ctx, account.ID, user.ID, 10, nil, nil)
	if err != nil {
		t.Fatalf("list by owner: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(listed))
	}

	needle := "shared-search-token-unique"
	for _, id := range []uuid.UUID{threadA.ID, threadB.ID} {
		if _, err := messageRepo.Create(ctx, account.ID, id, "user", needle, &user.ID); err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	searchHits, err := threadRepo.SearchByQuery(ctx, account.ID, user.ID, needle, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(searchHits) != 2 {
		t.Fatalf("expected 2 search hits, got %#v", searchHits)
	}

	forkSource, err := messageRepo.Create(ctx, account.ID, threadB.ID, "user", "fork source body", &user.ID)
	if err != nil {
		t.Fatalf("create fork source message: %v", err)
	}
	forked, err := threadRepo.Fork(ctx, account.ID, &user.ID, threadB.ID, forkSource.ID, false)
	if err != nil {
		t.Fatalf("fork thread: %v", err)
	}
	if forked.ParentThreadID == nil || *forked.ParentThreadID != threadB.ID {
		t.Fatalf("expected parent_thread_id=%s, got %#v", threadB.ID, forked.ParentThreadID)
	}
	if forked.BranchedFromMessageID == nil || *forked.BranchedFromMessageID != forkSource.ID {
		t.Fatalf("expected branched_from_message_id=%s", forkSource.ID)
	}
}
