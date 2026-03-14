package kafka

import (
	"errors"
	"testing"
)

type fakeMetadataClient struct {
	topics        []string
	partitions    map[string][]int32
	refreshErr    error
	topicsErr     error
	partitionsErr map[string]error
}

func (f *fakeMetadataClient) RefreshMetadata(_ ...string) error {
	return f.refreshErr
}

func (f *fakeMetadataClient) Topics() ([]string, error) {
	return f.topics, f.topicsErr
}

func (f *fakeMetadataClient) Partitions(topic string) ([]int32, error) {
	if err := f.partitionsErr[topic]; err != nil {
		return nil, err
	}
	return f.partitions[topic], nil
}

func TestVerifyTopicsSuccess(t *testing.T) {
	t.Parallel()

	client := &fakeMetadataClient{
		topics: []string{TopicRoomMessages, TopicDMMessages, TopicPresence},
		partitions: map[string][]int32{
			TopicRoomMessages: {0},
			TopicDMMessages:   {0},
			TopicPresence:     {0},
		},
		partitionsErr: map[string]error{},
	}

	if err := verifyTopics(client, RequiredTopics()); err != nil {
		t.Fatalf("verify topics failed: %v", err)
	}
}

func TestVerifyTopicsMissingTopic(t *testing.T) {
	t.Parallel()

	client := &fakeMetadataClient{
		topics: []string{TopicRoomMessages, TopicDMMessages},
		partitions: map[string][]int32{
			TopicRoomMessages: {0},
			TopicDMMessages:   {0},
		},
		partitionsErr: map[string]error{},
	}

	err := verifyTopics(client, RequiredTopics())
	if err == nil {
		t.Fatal("expected verify topics to fail")
	}
	if err.Error() != "missing required topics: [chat.presence]" {
		t.Fatalf("unexpected verify topics error: %v", err)
	}
}

func TestConsumerReadyError(t *testing.T) {
	t.Parallel()

	consumer := &Consumer{}
	errExpected := errors.New("boom")
	consumer.setReadyError(errExpected)
	if err := consumer.ReadyError(); err != errExpected {
		t.Fatalf("expected ready error %v, got %v", errExpected, err)
	}
	consumer.setReadyError(nil)
	if err := consumer.ReadyError(); err != nil {
		t.Fatalf("expected nil ready error, got %v", err)
	}
}
