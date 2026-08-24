package database

import (
	"context"
	"testing"
	"time"
)

func TestRoomMembershipAndChat(t *testing.T) {
	store, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	owner, err := store.CreateUser(ctx, "owner@example.com", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	member, err := store.CreateUser(ctx, "member@example.com", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	video := Video{ID: "video", OwnerID: owner.ID, OriginalName: "clip.mp4", StorageKey: "clip.mp4", ContentType: "video/mp4", CreatedAt: time.Now().UTC()}
	if err := store.CreateVideo(ctx, video); err != nil {
		t.Fatal(err)
	}
	room := Room{ID: "room", HostID: owner.ID, VideoID: video.ID, Title: "Test", InviteToken: "invite", InviteExpiresAt: time.Now().UTC().Add(24 * time.Hour), Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := store.CreateRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	if _, err := store.JoinRoom(ctx, room.InviteToken, member.ID); err != nil {
		t.Fatal(err)
	}
	message, err := store.AddChatMessage(ctx, room.ID, member.ID, "hello")
	if err != nil || message.SenderName != "member" {
		t.Fatalf("unexpected chat message: %#v, %v", message, err)
	}
	history, err := store.ChatHistory(ctx, room.ID, owner.ID, 50)
	if err != nil || len(history) != 1 || history[0].Body != "hello" {
		t.Fatalf("unexpected history: %#v, %v", history, err)
	}
}
