/* eslint-disable no-unused-vars */
//  const mqtt = require('mqtt');

let bJoind = false;
let bPushAuthed = false;
let bHeartbeatStarted = false;
let evnet_callback_;
let sessionid_;
let keepalive_timer_id_;
let request_timer_id_;
let mqtt_client;
const nickname_ = "二狗子";// window.localStorage.getItem('nickname_');
const roomid_ = "Allison";// window.localStorage.getItem('roomid');
const mqtt_topic_ = "Catherine";// window.localStorage.getItem('mqtt_topic');

function mqtt_init(clientid, callback) {
  sessionid_ = clientid;
  if (typeof callback === 'function') {
    evnet_callback_ = callback;
  }
  const options = {
    keepalive: 30,
    clientId: sessionid_,
    protocolId: 'MQTT',
    protocolVersion: 4,
    clean: true,
    reconnectPeriod: 3*1000,
    connectTimeout: 2 * 1000,
    will: {
      topic: 'client_disconn',
      payload: 'client:%s disconnect',
      qos: 0,
      retain: false
    },
    username: 'admin',
    password: 'public',
    rejectUnauthorized: false
  };
  const connectUrl = 'ws://127.0.0.1:8083/mqtt';
  mqtt_client = mqtt.connect(connectUrl, options);

  // subscribe topic
  mqtt_client.subscribe(sessionid_);

  //  event monitor
  mqtt_client.on('connect', (packet) => {
    console.log('mqtt connected...');
    evnet_callback_({type: 'mqtt_connected', info: ''});
  });
  mqtt_client.on('reconnect', (error) => {
    evnet_callback_({type: 'mqtt_disconnected', info: ''});
  });
  mqtt_client.on('error', (error) => {
    evnet_callback_({type: 'mqtt_disconnected', info: ''});
  });
  mqtt_client.on('message', (topic, message) => {
    console.log('receive message：%s', message.toString());
    responseHandler(message);
  });
}

//  generate uuid
function generateUUID() {
  let d = new Date().getTime();
  if (window.performance && typeof window.performance.now === 'function') {
    d += performance.now(); //  use high-precision timer if available
  }
  const uuid = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
    const r = (d + Math.random() * 16) % 16 | 0;
    d = Math.floor(d / 16);
    return (c == 'x' ? r : (r & 0x3 | 0x8)).toString(16);
  });
  return uuid;
}

//  regist to media server
async function join2ms() {
  console.log('start join room...');
  const browserType = getBrowserType();
  const request={
    type: 'login',
    roomid: roomid_,
    sessionid: sessionid_,
    username: nickname_,
    devtype: 'OffcnLiveMC/web',
    subtype: browserType,
    version: '0.0.1'
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  keepalive
async function keepAlive() {
  const request={
    type: 'heartbeat',
    sessionid: sessionid_,
    roomid: roomid_
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  try start publish stream to media server
async function authPush(streamid) {
  const request={
    type: 'publish',
    sessionid: sessionid_,
    roomid: roomid_,
    peerid: streamid
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  unpublish stream
async function unpush(sid) {
  const request={
    type: 'unpush',
    sessionid: sessionid_,
    roomid: roomid_,
    peerid: sid
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  offer for publish stream
async function offer(sid, sdp) {
  const request={
    type: 'offer',
    sessionid: sessionid_,
    roomid: roomid_,
    peerid: sid,
    sdp: sdp
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  answer for subscribe stream
async function answer(fid, tid, sid, sdp) {
  const request={
    type: 'answer',
    sessionid: sessionid_,
    roomid: roomid_,
    peerid: sid,
    sdp: sdp
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  answer for subscribe stream
async function candidate(candi,peerid) {
  const request={
    type: 'candidate',
    sessionid: sessionid_,
    roomid: roomid_,
    peerid: peerid,
    candidate: candi,
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  subscribe stream
async function sub(clientid, streamid) {
  const request={
    type: 'sub',
    roomid: roomid_,
    sessionid: sessionid_,
    peerid: streamid
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  stop subscribe stream
async function unsub(clientid, streamid) {
  const request={
    type: 'unsub',
    roomid: roomid_,
    sessionid: sessionid_,
    peerid: streamid,
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

function responseHandler(msg) {
  const json = JSON.parse(msg);
  if (json.type == 'login') {
    joinHandler(json);
  } else if (json.type == 'pub') {
    pubHandler(json);
  } else if (json.type == 'unpub') {
    unpubHandler(json);
  } else if (json.type == 'heartbeat') {
    heartbeatHandler(json);
  } else if (json.type == 'publish') {
    pushHandler(json);
  } else if (json.type == 'offer') {
    offerHandler(json);
  } else if (json.type == 'answer') {
    answerHandler(json);
  } else if (json.type == 'candidate') {
    candidateHandler(json);
  } else if (json.type == 'unpush') {
    unpushHandler(json);
  } else if (json.type == 'sub') {
    subHandler(json);
  } else if (json.type == 'unsub') {
    unsubHandler(json);
  } else if (json.type == 'logout') {
    logoutHandler(json);
  } else {
    console.log('unknow message：', json.type);
  }
}

function joinHandler(msg) {
  if (msg.code == '0') {
    joinButton.disabled = true;
    bHeartbeatStarted = true;
    keepalive_timer_id_ = window.setInterval(function() {
      keepAlive();
    }, 30*1000);
    bJoind = true;
    const event = {
      type: 'join_succeed',
      info: ''
    };
    evnet_callback_(event);
    console.log('join successed');
  }
}

function pubHandler(msg) {
  console.log('%s-%s published', msg.sessionid, msg.peerid);
  const event = {
    type: 'pub',
    info: msg
  };
  evnet_callback_(event);
}

function unpubHandler(msg) {
  console.log('%s-%s unpublished', msg.sessionid, msg.peerid);
  const event = {
    type: 'unpub',
    info: msg
  };
  evnet_callback_(event);
}

function heartbeatHandler(msg) {
  if (msg.result!=1) {
    console.log('not joined, need to join first');
    //  should stop keepalive timer
    clearInterval(keepalive_timer_id_);
    join2ms(nickname_, roomid_);
    bJoind = false;

    //  TODO: need to clean other flags and streams.
  } else {
    alert('join failed! try again maybe later?');
  }
}

function pushHandler(msg) {
  if (msg.code == '0') {
    bPushAuthed = true;
    const event = {
      type: 'push_succeed',
      info: msg
    };
    evnet_callback_(event);
  }
}

//  subscribe response
function offerHandler(msg) {
  //  TODO: set remote sdp, then send answer
  const event = {
    type: 'recv_offer',
    info: msg
  };
  evnet_callback_(event);
}

//  publish response
function answerHandler(msg) {
  //  TODO: set remote sdp
  const event = {
    type: 'answer_succeed',
    info: msg
  };
  evnet_callback_(event);
}

function candidateHandler(msg) {
  //  TODO: set candidate
  const event = {
    type: 'recv_candidate',
    info: msg
  };
  evnet_callback_(event);
}

function unpushHandler(msg) {

}

function logoutHandler(msg) {

}


function getBrowserType() {
  const userAgent = navigator.userAgent;
  const isOpera = userAgent.indexOf('Opera') > -1;
  const isIE = userAgent.indexOf('compatible') > -1 && userAgent.indexOf('MSIE') > -1 && !isOpera;
  const isEdge = userAgent.indexOf('Edge') > -1;
  const isFF = userAgent.indexOf('Firefox') > -1;
  const isSafari = userAgent.indexOf('Safari') > -1 && userAgent.indexOf('Chrome') == -1;
  const isChrome = userAgent.indexOf('Chrome') > -1 && userAgent.indexOf('Safari') > -1;

  if (isIE) {
    const reIE = new RegExp('MSIE (\\d+\\.\\d+);');
    reIE.test(userAgent);
    const fIEVersion = parseFloat(RegExp['$1']);
    if (fIEVersion == 7) {
      return 'IE7';
    } else if (fIEVersion == 8) {
      return 'IE8';
    } else if (fIEVersion == 9) {
      return 'IE9';
    } else if (fIEVersion == 10) {
      return 'IE10';
    } else if (fIEVersion == 11) {
      return 'IE11';
    } else {
      return '0';
    }
    return 'IE';
  }
  if (isOpera) {
    return 'Opera';
  }
  if (isEdge) {
    return 'Edge';
  }
  if (isFF) {
    return 'FF';
  }
  if (isSafari) {
    return 'Safari';
  }
  if (isChrome) {
    return 'Chrome';
  }
}