/* eslint-disable no-extend-native */

// const mqtt_moudle=document.createElement('script');
// mqtt_moudle.setAttribute('type', 'text/javascript');
// mqtt_moudle.setAttribute('src', 'js/mqttprocesser.js');
// document.body.appendChild(mqtt_moudle);

const audio_input_select = document.getElementById('audio_input_devices');
const video_capture_select = document.getElementById('video_capture_devices');

let mqtt_connected_ = false; //  mqtt connect status
let joined_room_ = false; //  join room status
const usermap_ = new Map(); //  users set
const publish_map_ = new Map(); //  publish streams set
const subscribe_map_ = new Map(); //  subscribe streams set
let usercallback;

function textareaHandler(e) {
  if (e.keyCode == 13) {
    if (e.ctrlKey || e.altKey || e.shiftKey || e.metaKey) {
      document.getElementById("text-sendbuf").value = document.getElementById("text-sendbuf").value + '\n';
    } else {
      e.preventDefault();
      
      var isSend = false;
      const key = clientid_+'_'+peerid_;
      if (publish_map_.has(key)) {
        publish_map_.get(key).data_channel.send(document.getElementById("text-sendbuf").value);
        isSend = true;
      }

      if (isSend == false) {
        for (var x of subscribe_map_) {
          if (x[1].sub_data_channel != null) {
            x[1].sub_data_channel.send(document.getElementById("text-sendbuf").value);
            isSend = true;
            break;
          }
        }
      }

      if (isSend) {
        updateRecvBuffer("send", document.getElementById("text-sendbuf").value)
      }
      document.getElementById("text-sendbuf").value = ""
    }
  }
}

function updateRecvBuffer(type, data) {
  var recvbuf = document.getElementById("text-recvbuf");
  recvbuf.value = recvbuf.value + type + ":" + data + "\n";
}

Array.prototype.indexOf = function(val) {
  for (let i = 0; i < this.length; i++) {
    if (this[i] == val) {
      return i;
    }
  }
  return -1;
};
Array.prototype.remove = function(val) {
  const index = this.indexOf(val);
  if (index > -1) {
    this.splice(index, 1);
  }
};

//  for notify UI event
function set_user_event_callback(eCallback) {
  if (typeof eCallback === 'function') {
    // eslint-disable-next-line no-unused-vars
    usercallback = eCallback;
  }
}

function device_discovery() {
  if (!navigator.mediaDevices && !navigator.mediaDevices.enumerateDevices) {
    console.log('The browser is not surpport enum media device');
  } else {
    navigator.mediaDevices.addEventListener('devicechange', deviceChange);
    navigator.mediaDevices.enumerateDevices().then(gotDevices).catch(enumError);
  }
}

function gotDevices(deviceInfos) {
  for (const info of deviceInfos) {
    const event = {
      id: 'device',
      info: info
    };
    usercallback(event);
  }
}

function enumError(e) {
  console.log('error:'+e);
}

function deviceChange(e) {
  const event = {
    id: 'device-change',
    info: ''
  };
  usercallback(event);
}

clientid_ = generateUUID(); //'mqttjs_' + Math.random().toString(16).substr(2, 8);
var peerid_; //for data channel search
mqtt_init(clientid_, mqttEventCallback);

//  join the room of media server
async function join_room() {
  if (mqtt_connected_) {
    join2ms(window.document.getElementById('edit-nickname').value,window.document.getElementById('edit-roomid').value);
  } else {
    console.log('not connect to mqtt, please try join room later...');
  }
}

//  leave the room of media server
async function leave_room() {
  if (mqtt_connected_) {
    //  leave room
    joined_room_ = false;
  } else {
    console.log('not connect to mqtt, please try leave room later...');
  }
}

//  publish stream
async function publish_local_stream(peerid, videolable) {
  if (mqtt_connected_ && joined_room_) {
    if (publish_map_.has(clientid_+'_'+peerid)) {
      console.log(peerid+' stream has been published and would be republished now');
      stopPublish(peerid);
    }
    const node={
      sid: peerid,
      peer_conn: null,
      data_channel: null,
      video_wnd: videolable
    };
    publish_map_.set(clientid_+'_'+peerid, node);
    authPush(peerid);
  } else {
    console.log('not connect to mqtt or not join, please try publish stream later...');
  }
}

//  unpublish stream
async function unpublish_local_stream(peerid) {
  if (mqtt_connected_ && joined_room_) {
    stopPublish(peerid);
  } else {
    console.log('not connect to mqtt, please try unpublish stream later...');
  }
}

//  publish stream
async function publish_screenshare(peerid, videolable) {
  if (publish_map_.has(clientid_+'_'+peerid)) {
    console.log(peerid+' stream has been published and would be republished now');
    stopPublish(peerid);
  }
  if (mqtt_connected_ && joined_room_) {
    const node={
      sid: peerid,
      peer_conn: null,
      video_wnd: videolable
    };
    publish_map_.set(clientid_+'_'+peerid, node);
    authPush(peerid);
  } else {
    console.log('not connect to mqtt, please try publish stream later...');
  }
}

//  unpublish stream
async function unpublish_screenshare(peerid) {
  if (mqtt_connected_ && joined_room_) {
    stopPublish(peerid);
  } else {
    console.log('not connect to mqtt, please try unpublish stream later...');
  }
}

//  subscribe audio stream
async function subscribe_remote_stream(userid, peerid, videolable) {
  if (mqtt_connected_ && joined_room_) {
    for (const [key, value] of usermap_) {
      if (key!=clientid_) {
        for (let i =0; i < value.length; i++) {
          if (value[i]===peerid) {
            subscribe(key, value[i], videolable);
            break;
          }
        }
        return;
      }
    }
  } else {
    console.log('not connect to mqtt, please try subscribe stream later...');
  }
}

//  unsubscribe audio stream
async function unsubscribe_remote_stream(peerid) {
  if (mqtt_connected_ && joined_room_) {
    if (mqtt_connected_ && joined_room_) {
      for (const [key, value] of usermap_) {
        if (key!=clientid_) {
          for (let i =0; i < value.length; i++) {
            if (value[i]===peerid) {
              stopPull(key, value[i]);
              break;
            }
          }
          return;
        }
      }
    }
  } else {
    console.log('not connect to mqtt, please try unsubscribe stream later...');
  }
}

async function swap_position(videolable1, videolable2) {
  let swap1=null;
  let swap2=null;
  for ([key, value] of publish_map_) {
    if (value.video_wnd == videolable1) {
      swap1 = value;
    }
    if (value.video_wnd == videolable2) {
      swap2 = value;
    }
  }
  for ([key, value] of subscribe_map_) {
    if (value.video_wnd == videolable1) {
      swap1 = value;
    }
    if (value.video_wnd == videolable2) {
      swap2 = value;
    }
  }
  if (swap1!=null && swap2!=null) {
    const tmp = swap1.video_wnd;
    swap1.video_wnd = swap2.video_wnd;
    swap2.video_wnd = tmp;
  }
}

async function mqttEventCallback(event) {
  if (event.type=='mqtt_connected') {
    mqtt_connected_ = true;
  } else if (event.type=='mqtt_disconnected') {
    mqtt_connected_ = false;
    //  TODO:do something
  } else if (event.type=='join_succeed') {
    joined_room_ = true;
    window.document.getElementById('clientid').textContent = sessionid_;
  } else if (event.type=='join_failed') {
    joined_room_ = false;
  } else if (event.type=='pub') {
    handlePub(event.info);
  } else if (event.type=='unpub') {
    handleUnpub(event.info);
  } else if (event.type=='push_succeed') {
    for ([key, value] of publish_map_) {
      if (value.peer_conn==null) {
        startPublishOffer(event.info, value.sid);
      }
    }
  } else if (event.type=='push_failed') {
    //  TODO:
  } else if (event.type=='recv_answer') {
    publishAnswerHandler(event.info);
  } else if (event.type=='recv_offer') {
    //  sub response
    subOfferHandler(event.info);
  } else if (event.type=='recv_candidate') {
    handleRemoteCandi(event.info);
  } else {
    console.log("unknow event type:%s",event.type);
  }
}

function getScreenShareConstraints() {
  const videoConstraints = {};
  videoConstraints.aspectRatio = '1.77';//  1.77 means 16:9
  videoConstraints.frameRate = '15'; // 15 frames/sec
  videoConstraints.cursor = 'always'; //  never motion
  videoConstraints.displaySurface = 'monitor';//  monitor window application browser
  videoConstraints.logicalSurface = true;
  // videoConstraints.width = screen.width;
  // videoConstraints.height = screen.height;
  videoConstraints.width = 640;
  videoConstraints.height = 480;

  if (!Object.keys(videoConstraints).length) {
    videoConstraints = true;
  }

  const displayMediaStreamConstraints = {
    video: videoConstraints,
  };
  return displayMediaStreamConstraints;
}

async function startPublishOffer(msg, peerid) {
  let stream;
  let peerOpt;
  peerid_ = peerid;
  try {
    if (peerid=='window') {
      const opt = getScreenShareConstraints();
      stream = await navigator.mediaDevices.getDisplayMedia(opt);
      //  peerOpt = {sdpSemantics: 'plan-b'};
      peerOpt = {
        iceServers: [{
            urls: window.document.getElementById('edit-stunurl').value,// "turn:node.offcncloud.com:9900",
            username: window.document.getElementById('edit-stun-userid').value,// "ctf",
            credential: window.document.getElementById('edit-stun-pwd').value,// "ctf123"
        }],
        iceTransportPolicy: "all",
        sdpSemantics: 'unified-plan'
      };
    } else {
      const media_option = {
        audio: {
          noiseSuppression: true,
          echoCancellation: true,
          deviceId: audio_input_select.options[audio_input_select.selectedIndex].value
        },
        video: {
          width: 640,
          height: 480,
          frameRate: 15,
          deviceId: video_capture_select.options[video_capture_select.selectedIndex].value
        }
      };
      stream = await navigator.mediaDevices.getUserMedia(media_option);// {audio: true, video: true}
      peerOpt = {
        iceServers: [{
          urls: window.document.getElementById('edit-stunurl').value,// "turn:node.offcncloud.com:9900",
          username: window.document.getElementById('edit-stun-userid').value,// "ctf",
          credential: window.document.getElementById('edit-stun-pwd').value,// "ctf123"
        }],
        iceTransportPolicy: "all",
        sdpSemantics: 'unified-plan'
      };
    }
  } catch (e) {
    publish_map_.delete(clientid_+'_'+peerid);
    alert(`getUserMedia() error: ${e.name}`);
  }
  
  const key = msg.sessionid+'_'+peerid;
  if (publish_map_.has(key)) {
    publish_map_.get(key).video_wnd.srcObject = stream;
  } else {
    console.log("startPublishOffer return");
    return;
  }

  startTime = window.performance.now();
  const videoTracks = stream.getVideoTracks();
  const audioTracks = stream.getAudioTracks();
  if (videoTracks.length > 0) {
    console.log(`Using video device: ${videoTracks[0].label}`);
  }
  if (audioTracks.length > 0) {
    console.log(`Using audio device: ${audioTracks[0].label}`);
  }
  peer = new RTCPeerConnection(peerOpt);
  var channel = peer.createDataChannel("datachannel");
  channel.onopen = handleChannelStatusChange;
  channel.onclose = handleChannelStatusChange;
  channel.onmessage = handleRecvSubChannelMsg;
  if (publish_map_.has(key)) {
    publish_map_.get(key).peer_conn = peer;
    publish_map_.get(key).data_channel = channel;
  } else {
    return;
  }

  // 向对方发送nat candidate
  peer.onicecandidate = event => {
    if (!event.candidate) {
        return;
    }
    console.log('RTCPeerConnection callback candidate:', event.candidate);
    candidate(peerid,event.candidate)
  };

  peer.addEventListener('iceconnectionstatechange', e => onIceStateChange(peer, e));

  stream.getTracks().forEach(track => peer.addTrack(track, stream));

  try {
    const offer_sdp = await peer.createOffer({offerToReceiveAudio: 1, offerToReceiveVideo: 1});
    peer.setLocalDescription(offer_sdp);
    offer(peerid, offer_sdp.sdp);
  } catch (e) {
    console.log('Failed to create sdp: ${e.toString()}');
  }
}

async function publishAnswerHandler(msg) {
  const answer_sdp = {
    sdp: msg.sdp,
    type: 'answer'
  };

  try {
    const key = msg.sessionid+'_'+msg.peerid;
    const peer = publish_map_.get(key).peer_conn;
    peer.setRemoteDescription(answer_sdp);
  } catch (e) {
    console.log(`setRemoteDescription failed: ${e.toString()}`);
  }
}

function handlePub(msg) {
  if (!usermap_.has(msg.sessionid)) {
    const list = [];
    list.push(msg.peerid);
    usermap_.set(msg.sessionid, list);
  } else {
    usermap_.get(msg.sessionid).push(msg.peerid);
  }
}

function handleUnpub(msg) {
  if (usermap_.has(msg.sessionid)) {
    usermap_.get(msg.sessionid).remove(msg.peerid);
  }
}

function subscribe(userid, peerid, videolable) {
  if (subscribe_map_.has(userid+'_'+peerid)) {
    console.log(peerid+' stream has been subscribed and would be resubscribed now');
    stopPull(key, peerid);
  }
  peer = new RTCPeerConnection({sdpSemantics: 'unified-plan'}); // {sdpSemantics: "unified-plan"}
  const sem = peer.getConfiguration().sdpSemantics;
  console.log('pull peer semantics:'+sem);
  const node={
    peer_conn: null,
    sub_data_channel: null,
    video_wnd: videolable
  };
  node.peer_conn = peer;
  subscribe_map_.set(userid+'_'+peerid, node);
  // 向对方发送nat candidate
  peer.onicecandidate = event => {
    if (!event.candidate) {
        return;
    }
    console.log('RTCPeerConnection callback candidate:', event.candidate);
    sub_candidate(userid,peerid,event.candidate)
  };

  peer.ondatachannel = function (e) {
    node.sub_data_channel = e.channel;
    node.sub_data_channel.onmessage = handleRecvSubChannelMsg;
    node.sub_data_channel.onopen = handleChannelStatusChange;
    node.sub_data_channel.onclose = handleChannelStatusChange;
  }
  peer.addEventListener('iceconnectionstatechange', e => onIceStateChange(peer, e));
  peer.addEventListener('track', e => gotRemoteStream(userid, peerid, e));
  sub(userid, peerid);
}

function handleRecvSubChannelMsg(e) {
  updateRecvBuffer("recv",e.data);
}

function handleChannelStatusChange(e) {
  if (e.type == "open") {
    document.getElementById("text-sendbuf").disabled = false;
  } else if (e.type == "close") {
    document.getElementById("text-sendbuf").disabled = true;
  }
}

async function subOfferHandler(msg) {
  const offer_sdp = {
    sdp: msg.sdp,
    type: 'offer'
  };
  const key = msg.srcsessionid+'_'+msg.peerid;
  const peer = subscribe_map_.get(key).peer_conn;
  peer.setRemoteDescription(offer_sdp);
  stopPullButton.disabled = false;
  try {
    const answerOptions = {
      offerToReceiveAudio: 1,
      offerToReceiveVideo: 1
    };
    const answersdp = await peer.createAnswer(answerOptions);
    peer.setLocalDescription(answersdp);
    answer(msg.srcsessionid, msg.peerid, answersdp.sdp);
  } catch (e) {
    console.log(`Failed to create sdp: ${e.toString()}`);
  }
}

async function handleRemoteCandi(msg) {
  var candiInit = JSON.parse(msg.candidate);
  const candi = new RTCIceCandidate(candiInit);
  await peer.addIceCandidate(candi);
}

function gotRemoteStream(userid, peerid, e) {
  if (remoteVideo.srcObject !== e.streams[0]) {
    subscribe_map_.get(userid+'_'+peerid).video_wnd.srcObject = e.streams[0];
    console.log('%s_%s received remote stream', userid, peerid);
  }
}

function onIceStateChange(peer, event) {
  if (peer) {
    console.log(` ICE state: ${peer.iceConnectionState}`);
    console.log('ICE state change event: ', event);
  }
}

function stopPublish(peerid) {
  const key = clientid_+'_'+peerid;
  const peer = publish_map_.get(key).peer_conn;
  //  peer.stream.getTracks().forEach(track => track.stop());
  publish_map_.get(key).video_wnd.srcObject = null;
  peer.close();
  publish_map_.get(key).peer_conn = null;
  publish_map_.delete(key);
  unpush(peerid);
}

function stopPull(userid, peerid) {
  const key = userid+'_'+peerid;
  const peer = subscribe_map_.get(key).peer_conn;
  //  peer.stream.getTracks().forEach(track => track.stop());
  subscribe_map_.get(key).video_wnd.srcObject = null;
  peer.close();
  subscribe_map_.get(key).peer_conn = null;
  subscribe_map_.delete(key);
  unsub(userid, peerid);
}
