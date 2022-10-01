/* eslint-disable no-unused-vars */
//  const mqtt = require('mqtt');

let bJoind = false;
let bPushAuthed = false;
let bHeartbeatStarted = false;
let evnet_callback_;
let userid_;
let keepalive_timer_id_;
let request_timer_id_;
let mqtt_client;
let nickname_ ;
let roomid_;
let mqtt_topic_;

function mqtt_init(callback) {
  mqttid = generateUUID();
  if (typeof callback === 'function') {
    evnet_callback_ = callback;
  }
  const options = {
    keepalive: 30,
    clientId: mqttid,
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
  const connectUrl = 'wss://test.nb666.com:8084/mqtt';
  mqtt_client = mqtt.connect(connectUrl, options);

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
async function join2ms(nickname, clientid, roomid, mqtt_topic) {
  // subscribe response topic
  userid_ = clientid;
  mqtt_client.subscribe(userid_);

  nickname_ = nickname;
  roomid_ = roomid;
  mqtt_topic_ = mqtt_topic;
  console.log('start join room...');
  const browserType = getBrowserType();
  const request={
    cmd: 'login',
    roomid: roomid_,
    userid: userid_,
    username: nickname_,
    devtype: 'chrome/web',
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
    cmd: 'heartbeat',
    userid: userid_,
    roomid: roomid_
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  try start publish stream to media server
async function authPush(streamid) {
  const request={
    cmd: 'publish',
    userid: userid_,
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
    cmd: 'unpush',
    userid: userid_,
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
    cmd: 'offer',
    userid: userid_,
    roomid: roomid_,
    peerid: sid,
    sdp: sdp
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  answer for subscribe stream
async function answer(ssid, pid, sdp) {
  const request={
    cmd: 'answer',
    roomid: roomid_,
    userid: userid_,
    remoteuserid: ssid,
    peerid: pid,
    sdp: sdp
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  answer for subscribe stream
async function candidate(pid,candi) {
  const request={
    cmd: 'candidate',
    roomid: roomid_,
    userid: userid_,
    peerid: pid,
    candidate: candi,
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  answer for subscribe stream
async function sub_candidate(ssid,pid,candi) {
  const request={
    cmd: 'candidate',
    roomid: roomid_,
    userid: userid_,
    remoteuserid: ssid,
    peerid: pid,
    candidate: candi,
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  subscribe stream
async function sub(clientid, streamid) {
  const request={
    cmd: 'sub',
    roomid: roomid_,
    userid: userid_,
    remoteuserid:clientid,
    peerid: streamid
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

//  stop subscribe stream
async function unsub(clientid, streamid) {
  const request={
    cmd: 'unsub',
    roomid: roomid_,
    userid: userid_,
    peerid: streamid,
  };
  const jsonText=JSON.stringify(request);
  console.log('send msg:%s %s', mqtt_topic_, jsonText);
  mqtt_client.publish(mqtt_topic_, jsonText);
}

function responseHandler(msg) {
  const json = JSON.parse(msg);
  if (json.cmd == 'login') {
    joinHandler(json);
  } else if (json.cmd == 'pub') {
    pubHandler(json);
  } else if (json.cmd == 'unpub') {
    unpubHandler(json);
  } else if (json.cmd == 'heartbeat') {
    heartbeatHandler(json);
  } else if (json.cmd == 'publish') {
    pushHandler(json);
  } else if (json.cmd == 'offer') {
    offerHandler(json);
  } else if (json.cmd == 'answer') {
    answerHandler(json);
  } else if (json.cmd == 'candidate') {
    candidateHandler(json);
  } else if (json.cmd == 'unpush') {
    unpushHandler(json);
  } else if (json.cmd == 'sub') {
    subHandler(json);
  } else if (json.cmd == 'unsub') {
    unsubHandler(json);
  } else if (json.cmd == 'logout') {
    logoutHandler(json);
  } else {
    console.log('unknow message：', json.cmd);
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
  console.log('%s-%s published', msg.userid, msg.peerid);
  const event = {
    type: 'pub',
    info: msg
  };
  evnet_callback_(event);
}

function unpubHandler(msg) {
  console.log('%s-%s unpublished', msg.userid, msg.peerid);
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
    join2ms(nickname_, userid_, roomid_, mqtt_topic_);
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
    type: 'recv_answer',
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