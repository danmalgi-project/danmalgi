package client

import (
	"context"
	"testing"

	user_storev1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/private/user_store/v1"
	userv1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public/user/v1"
)

// TestFakeUserStoreClientReturnsEveryRequestedID는 standalone 모드의 핵심 계약을 검증한다.
// AuthMiddleware가 토큰의 userId로 GetUserByID를 호출하고 UserService는 응답의 [0]을 그대로 읽으므로,
// 요청한 모든 userId에 대해 응답이 나와야 연결이 성립하고 인덱스 접근도 안전하다.
func TestFakeUserStoreClientReturnsEveryRequestedID(t *testing.T) {
	client := fakeUserStoreClient{}
	requested := []int64{1, 42, 1000000}

	response, err := client.GetUsers(context.Background(), &user_storev1.GetUsersRequest{UserIds: requested})
	if err != nil {
		t.Fatalf("GetUsers 실패: %v", err)
	}

	if len(response.GetUsers()) != len(requested) {
		t.Fatalf("응답 유저 수가 다르다: got %d, want %d", len(response.GetUsers()), len(requested))
	}

	for i, user := range response.GetUsers() {
		if user.GetId() != requested[i] {
			t.Errorf("[%d] 요청 순서와 id가 어긋난다: got %d, want %d", i, user.GetId(), requested[i])
		}
		if user.GetStatus() != int32(userv1.UserStatus_USER_ACTIVE) {
			t.Errorf("[%d] 유저가 활성 상태가 아니다: got %d", i, user.GetStatus())
		}
		if user.GetName() == "" || user.GetEmail() == "" {
			t.Errorf("[%d] 표시용 필드가 비어 있다: name=%q email=%q", i, user.GetName(), user.GetEmail())
		}
	}
}

// TestFakeUserIsDeterministic은 같은 userId가 언제나 같은 유저로 보이는지 확인한다.
// 재연결마다 표시 이름이 흔들리면 다중 접속 검증에서 참가자를 구분할 수 없다.
func TestFakeUserIsDeterministic(t *testing.T) {
	first := FakeUser(7)
	second := FakeUser(7)

	if first.GetName() != second.GetName() || first.GetEmail() != second.GetEmail() || first.GetTag() != second.GetTag() {
		t.Errorf("같은 userId가 다른 유저로 나왔다: %+v vs %+v", first, second)
	}

	if FakeUser(7).GetName() == FakeUser(8).GetName() {
		t.Errorf("서로 다른 userId가 같은 이름을 갖는다 — 참가자를 구분할 수 없다")
	}
}

// TestFakeUserStoreClientEmptyRequest는 빈 요청에 nil이 아닌 빈 응답이 나오는지 확인한다.
func TestFakeUserStoreClientEmptyRequest(t *testing.T) {
	response, err := fakeUserStoreClient{}.GetUsers(context.Background(), &user_storev1.GetUsersRequest{})
	if err != nil {
		t.Fatalf("GetUsers 실패: %v", err)
	}
	if response == nil {
		t.Fatal("응답이 nil이다")
	}
	if len(response.GetUsers()) != 0 {
		t.Errorf("빈 요청에 유저가 돌아왔다: %d건", len(response.GetUsers()))
	}
}
