'use strict';

//  for import mqttprocesser.js
// const clientsdk_moudle=document.createElement('script');
// clientsdk_moudle.setAttribute('type', 'text/javascript');
// clientsdk_moudle.setAttribute('src', 'js/ms_client.js');
// document.body.appendChild(clientsdk_moudle);

const joinButton = document.getElementById('joinButton');
const pushButton = document.getElementById('pushButton');
const pullButton = document.getElementById('pullButton');
const stopPublishButton = document.getElementById('stoppublishButton');
const stopPullButton = document.getElementById('stoppullButton');
const pushdocButton = document.getElementById('pushdocButton');
const stoppushdocButton = document.getElementById('stoppushdocButton');

const localVideo = document.getElementById('localVideo');
const remoteVideo = document.getElementById('remoteVideo');
const docVideo = document.getElementById('docVideo');

joinButton.addEventListener('click', join_room);

pushButton.addEventListener('click', e => publish_local_stream('camera', localVideo));
pullButton.addEventListener('click', e => subscribe_remote_stream('', 'camera', remoteVideo));
pushdocButton.addEventListener('click', e => publish_screenshare('window', docVideo));

stopPublishButton.addEventListener('click', e => unpublish_local_stream('camera'));
stopPullButton.addEventListener('click', e => unsubscribe_remote_stream('camera'));
stoppushdocButton.addEventListener('click', e => unpublish_screenshare('window', docVideo));

localVideo.addEventListener('loadedmetadata', function() {
  console.log(`Local video videoWidth: ${this.videoWidth}px,  videoHeight: ${this.videoHeight}px`);
});

remoteVideo.addEventListener('loadedmetadata', function() {
  console.log(`Remote video videoWidth: ${this.videoWidth}px,  videoHeight: ${this.videoHeight}px`);
});

remoteVideo.addEventListener('resize', () => {
  console.log(`Remote video size changed to ${remoteVideo.videoWidth}x${remoteVideo.videoHeight}`);
});

docVideo.addEventListener('loadedmetadata', function() {
  console.log(`Remote video videoWidth: ${this.videoWidth}px,  videoHeight: ${this.videoHeight}px`);
});

set_user_event_callback(webrtc_event_monitor);
device_discovery();
const audio_input_map_ = new Map();
const video_input_map_ = new Map();
const audio_output_map_ = new Map();
let audio_default_input = '';
let audio_default_output = '';
async function webrtc_event_monitor(event) {
  if (event.id == 'device') {
    console.log(event.info.deviceId+'    '+event.info.groupId+'    '+event.info.kind+'    '+event.info.label);
    const info = {
      devid: event.info.deviceId,
      groupid: event.info.groupId,
      devname: event.info.label
    };
    if (event.info.kind=='audioinput') {
      if (event.info.deviceId=='default' || event.info.deviceId=='communications') {
        if (audio_default_input=='') {
          audio_default_input = event.info.groupId;
          set_audio_input_device(event.info.deviceId);
        }
      } else {
        audio_input_map_.set(event.info.groupId, info);
      }
    } else if (event.info.kind=='videoinput') {
      video_input_map_.set(event.info.groupId, info);
      if (event.info.label.search('Logitech') != -1) {
        set_video_input_device(event.info.deviceId);
      }
    } else if (event.info.kind=='audiooutput') {
      if (event.info.deviceId=='default' || event.info.deviceId=='communications') {
        if (audio_default_output=='') {
          audio_default_output = event.info.groupId;
          set_audio_output_device(event.info.deviceId);
        }
      } else {
        audio_output_map_.set(event.info.groupId, info);
      }
    }
  } else if (event.id == 'device-change') {
    audio_default_input = '';
    audio_default_output = '';
    audio_input_map_.clear();
    video_input_map_.clear();
    audio_output_map_.clear();
    device_discovery();
    console.log('device change');
  } else if (event.id == '') {

  } else if (event.id == '') {

  } else if (event.id == '') {

  } else if (event.id == '') {

  } else if (event.id == '') {

  }
}