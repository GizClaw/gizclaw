package rpcapi

func (t RPCPayload) AsFriendInfoGetRequest() (FriendInfoGetRequest, error) {
	var body FriendInfoGetRequest
	err := t.decode("FriendInfoGetRequest", &body)
	return body, err
}

func (t *RPCPayload) FromFriendInfoGetRequest(v FriendInfoGetRequest) error {
	return t.encode("FriendInfoGetRequest", v)
}

func (t *RPCPayload) MergeFriendInfoGetRequest(v FriendInfoGetRequest) error {
	return t.merge("FriendInfoGetRequest", v)
}

func (t RPCPayload) AsFriendInfoGetResponse() (FriendInfoGetResponse, error) {
	var body FriendInfoGetResponse
	err := t.decode("FriendInfoGetResponse", &body)
	return body, err
}

func (t *RPCPayload) FromFriendInfoGetResponse(v FriendInfoGetResponse) error {
	return t.encode("FriendInfoGetResponse", v)
}

func (t *RPCPayload) MergeFriendInfoGetResponse(v FriendInfoGetResponse) error {
	return t.merge("FriendInfoGetResponse", v)
}
