package main

const (
	CLUSTER_RECONNECTION_INTERVAL = 10 // units: second
	CLUSTER_RECONNECTION_MAXTIMES = 3

	CmdRoomCreated = "createroom"
	CmdHeartbeat   = "heartbeat"
	CmdLogin       = "login"
	CmdLogout      = "logout"
	CmdPush        = "publish"
	CmdStopPush    = "stoppush"
	CmdSub         = "sub"
	CmdStopSub     = "stopsub"
	CmdOffer       = "offer"
	CmdAnswer      = "answer"
	CmdCandidate   = "candidate"
	CmdPub         = "pub"
	CmdUnpub       = "unpub"

	CmdPodRegist         = "pod_regist"
	CmdPodRegistNow      = "regist_now"
	CmdPodReleasePublish = "unpush"
	CmdPodLeaveRoom      = "leaveroom"

	CmdFakePublish   = "fakepublish"
	CmdFakeUnpublish = "fakeunpublish"
	CmdFakeSubscribe = "fakesubscribe"
	CmdFakeOffer     = "fakeoffer"
	CmdFakeAnswer    = "fakeanswer"
	CmdFakeCandidate = "fakecandidate"

	LabelCmd       = "cmd"
	LabelRoomId    = "roomid"
	LabelClusterId = "clusterid"
	LabelPodId     = "podid"
	LabelSessID    = "userid"
	LabelSrcSessID = "remoteuserid"
	LabelPeerID    = "peerid"
	LabelSDP       = "sdp"
	LabelCode      = "code"
	LabelDesc      = "description"
	LabelSN        = "sn"
	LabelTimestamp = "timestamp"
	LabelResource  = "maxresource"

	CmdNotAvailable      = "notavailable"
	CmdNotLogin          = "notlogin"
	CmdNotFoundPublisher = "nopublisher"

	CodeNotAvailable          = 100
	DescNotAvailable          = "service is not available, please try again later"
	CodeNotLogin              = 101
	DescNotLogin              = "not login, please login first and try again"
	CodeNotFoundPublishUser   = 102
	DescNotFoundPublishUser   = "not found publish user"
	CodeNotFoundPublishPeer   = 103
	DescNotFoundPublishPeer   = "not found publish peer"
	CodeNotFoundSubscribePeer = 104
	DescNotFoundSubscribePeer = "not found subscribe peer"
	CodeCreatePeerError       = 105
	DescCreatePeerError       = "create peer failed"
)
