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

const audio_input_devices = document.getElementById('audio_input_devices');
const video_capture_devices = document.getElementById('video_capture_devices');
const audio_output_devices = document.getElementById('audio_output_devices');

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
let default_audio_input_groupid = '';
let default_audio_output_groupid = '';
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
        if (default_audio_input_groupid=='') {
          default_audio_input_groupid = event.info.groupId;
        }
      } else {
        audio_input_map_.set(event.info.deviceId, info);
        var isSelect = (default_audio_input_groupid==event.info.groupId);
        audio_input_devices.add(new Option(event.info.label,event.info.deviceId,isSelect));
      }
    } else if (event.info.kind=='videoinput') {
      video_input_map_.set(event.info.deviceId, info);
      // var isSelect = (event.info.label.search('Logitech') != -1);
      video_capture_devices.add(new Option(event.info.label,event.info.deviceId));
    } else if (event.info.kind=='audiooutput') {
      if (event.info.deviceId=='default' || event.info.deviceId=='communications') {
        if (default_audio_output_groupid=='') {
          default_audio_output_groupid = event.info.groupId;
        }
      } else {
        audio_output_map_.set(event.info.deviceId, info);
        var isSelect = (default_audio_output_groupid==event.info.groupId);
        audio_output_devices.add(new Option(event.info.label,event.info.deviceId,isSelect));
      }
    }
  } else if (event.id == 'device-change') {
    default_audio_input_groupid = '';
    default_audio_output_groupid = '';
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