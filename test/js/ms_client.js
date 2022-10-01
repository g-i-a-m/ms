/* eslint-disable no-extend-native */

// const mqtt_moudle=document.createElement('script');
// mqtt_moudle.setAttribute('type', 'text/javascript');
// mqtt_moudle.setAttribute('src', 'js/mqttprocesser.js');
// document.body.appendChild(mqtt_moudle);

var clientid_="";
var origin_clientid_="";
var peerid_; //for data channel search
const static_clientid = document.getElementById('clientid');
const audio_input_select = document.getElementById('audio_input_devices');
const video_capture_select = document.getElementById('video_capture_devices');
const checkbox_grcid = document.getElementById('check_grcid');
const checkbox = document.getElementById('check_cache');
checkbox_grcid.addEventListener("click", CheckboxGrcidClickedHandler);
checkbox.addEventListener("click", CheckboxClickedHandler);
InitLoadCache();
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
  recvbuf.scrollTop = recvbuf.scrollHeight;
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

function InitLoadCache() {
  id = localStorage.getItem('ms_demo_cache_clientid');
  if (id!=null && id.length!=0) {
    static_clientid.textContent = clientid_ = origin_clientid_ = id;
    checkbox.checked = true;
  }
  else {
    static_clientid.textContent = clientid_ = origin_clientid_ = generateUUID(); //'mqttjs_' + Math.random().toString(16).substr(2, 8);
  }
}

function CheckboxGrcidClickedHandler() {
  if (!checkbox_grcid.checked) {
    static_clientid.textContent = clientid_ = origin_clientid_;
  }
  else {
    static_clientid.textContent = clientid_ = generateUUID();
  }
}

function CheckboxClickedHandler() {
  if (!checkbox.checked) {
    if (window.confirm("you sure want to remote cached clientid? dummies")) {
      localStorage.removeItem('ms_demo_cache_clientid');
    }
  }
  else {
    if (window.confirm("you sure want to cache the clientid?")) {
      localStorage.setItem('ms_demo_cache_clientid',clientid_);
    }
  }
}


mqtt_init(mqttEventCallback);

//  join the room of media server
async function join_room() {
  if (mqtt_connected_) {
    var nickname = window.document.getElementById('edit-nickname').value;
    var roomid = window.document.getElementById('edit-roomid').value;
    var topic = window.document.getElementById('edit-cluster').value;
    join2ms(nickname, clientid_, roomid, topic);
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
async function subscribe_remote_stream(userid, peerid, videolable, audiolable) {
  if (mqtt_connected_ && joined_room_) {
    for (const [key, value] of usermap_) {
      if (key!=clientid_) {
        for (let i =0; i < value.length; i++) {
          if (value[i]===peerid) {
            subscribe(key, value[i], videolable, audiolable);
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
    static_clientid.textContent = userid_;
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
            urls: window.document.getElementById('edit-stunurl').value,// "turn:node.nb666.com:9900",
            username: window.document.getElementById('edit-stun-userid').value,// "ctf",
            credential: window.document.getElementById('edit-stun-pwd').value,// "ctf123"
        }],
        iceTransportPolicy: "all",
        sdpSemantics: 'unified-plan'
      };
    } else {
      const media_option = {
        audio: {
          // contentType: "audio/opus",
          noiseSuppression: true,
          echoCancellation: true,
          deviceId: audio_input_select.options[audio_input_select.selectedIndex].value
        },
        video: {
          // contentType: "video/webm;codec=h264",
          width: 320,
          height: 180,
          bitrate: 320000,
          frameRate: 15,
          deviceId: video_capture_select.options[video_capture_select.selectedIndex].value
        }
      };
      stream = await navigator.mediaDevices.getUserMedia(media_option);// {audio: true, video: true}
      peerOpt = {
        iceServers: [{
          urls: window.document.getElementById('edit-stunurl').value,// "turn:node.nb666.com:9900",
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
  
  const key = msg.userid+'_'+peerid;
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

  stream.getTracks().forEach(function(track){
    sender = peer.addTrack(track, stream);
    if (track.kind == 'video') {
      param = sender.getParameters();
      if (!param.encodings) {
        param.encodings = [{}];
        param.encodings[0].scaleResolutionDownBy = 1.0;
        param.encodings[0].maxBitrate = 320000;
      } else {
        param.encodings.forEach(function(encoding){
          encoding.maxBitrate = 320000;
        });
      }
      console.log("set maxBitrate = 320000");
      sender.setParameters(param);
      /* if (sender.getParameters().encodings[0].scaleResolutionDownBy == 1) {
        await sender.track.applyConstraints({height});
      } */
    }
  });

  try {
    const offer_sdp = await peer.createOffer({offerToReceiveAudio: 1, offerToReceiveVideo: 1});
    //set min bitrate
    offer_sdp.sdp = BandwidthHandler.setVideoBitrates(offer_sdp.sdp, {
      min: 256, //256 kbits
      max: 1000 //1000 kbits
    });
    peer.setLocalDescription(offer_sdp);
    offer(peerid, offer_sdp.sdp);
  } catch (e) {
    console.log('Failed to create sdp: ',e.toString());
  }
}

async function publishAnswerHandler(msg) {
  const answer_sdp = {
    sdp: msg.sdp,
    type: 'answer'
  };

  try {
    const key = msg.userid+'_'+msg.peerid;
    const peer = publish_map_.get(key).peer_conn;
    peer.setRemoteDescription(answer_sdp);
  } catch (e) {
    console.log(`setRemoteDescription failed: ${e.toString()}`);
  }
}

function handlePub(msg) {
  if (!usermap_.has(msg.userid)) {
    const list = [];
    list.push(msg.peerid);
    usermap_.set(msg.userid, list);
  } else {
    usermap_.get(msg.userid).push(msg.peerid);
  }
}

function handleUnpub(msg) {
  if (usermap_.has(msg.userid)) {
    usermap_.get(msg.userid).remove(msg.peerid);
  }
}

function subscribe(userid, peerid, videolable, audiolable) {
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
    video_wnd: videolable,
    audio_wnd: audiolable
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
  peer.ontrack = event => {
    console.log('%s_%s received remote %s track', userid, peerid, event.track.kind, event.streams);
    if (event.track.kind == 'audio') {
      rw = subscribe_map_.get(userid+'_'+peerid).audio_wnd;
      rw.srcObject = event.streams[0];
      rw.autoplay = true;
    }
    else if (event.track.kind == 'video') {
      rw = subscribe_map_.get(userid+'_'+peerid).video_wnd;
      rw.srcObject = event.streams[0];
      rw.autoplay = true;
    }
    
  };
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
  const key = msg.remoteuserid+'_'+msg.peerid;
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
    answer(msg.remoteuserid, msg.peerid, answersdp.sdp);
  } catch (e) {
    console.log(`Failed to create sdp: ${e.toString()}`);
  }
}

async function handleRemoteCandi(msg) {
  const candi = new RTCIceCandidate(msg.candidate);
  await peer.addIceCandidate(candi);
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
