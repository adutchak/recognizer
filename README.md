# Recognizer
A simple service used to recognize faces using AWS rekognition API. Supposed to be used in conjunction with Home Assistant

# RUN_MODE
The application can run in 2 modes: api and file_watcher. Below the description of each.

# RUN_MODE:file_watcher - api High level flow
1. Home Assistant makes WebRtc snapshot and locates it in the folder.
2. The folder where the snapshot is located, should be shared (SMB) with a host where we run this service. Then that folder is mounted as `/mnt/` into the app's container.
3. As soon as `TARGET_IMAGE_PATH` file is created, recognizer will start comparing it to images specified in `SAMPLE_IMAGE_PATHS`.
4. Base on recognition results, a message is pushes an MQTT message (`RECOGNIZED_MESSAGE`,`NOT_RECOGNIZED_MESSAGE`) to `MQTT_TOPIC`.

# RUN_MODE:api - api High level flow
1. Home Assistant makes a POST api call to the service and includes webrtc_url in the payload (example: `{"webrtc_url": "rtsp://username:password@192.168.1.120:554/cam/realmonitor?channel=1&subtype=0"}`). 
2. The recognizer takes the snapshot from the stream.
4. Base on recognition results, a message is pushes an MQTT message (`RECOGNIZED_MESSAGE`,`NOT_RECOGNIZED_MESSAGE`) to `MQTT_TOPIC`.

# Dependencies
Recognizer uses Amazon Rekognition service for detecting faces and labels. Therefore you should either mount AWS credentials into `/root/.aws/credentials` container's path, or use environment variables.

# Recognition configuration
Recognizer compares `TARGET_IMAGE_PATH` with all images specified in `SAMPLE_IMAGE_PATHS`. When at least one picture matches the target - it sends an MQTT message (`RECOGNIZED_MESSAGE`) to `MQTT_TOPIC`, then `TARGET_IMAGE_PATH` is deleted.   

During the recognition, the application uses `SIMILARITY_THRESHOLD`,  `CONFIDENCES_NOT_LESS_THAN` and `CONFIDENCES_NOT_MORE_THAN` parameters:   
`SIMILARITY_THRESHOLD`: https://docs.aws.amazon.com/rekognition/latest/APIReference/API_CompareFaces.html   
`CONFIDENCES_NOT_LESS_THAN` and `CONFIDENCES_NOT_MORE_THAN`:   
The application retrieves image labels (i.e. glasses, hat, floor etc). Each retrieved label has it's Confidence. You can set your requirements for these label's confidence. For example, you might not want someone trying to fake the snapshot image with showing the copy on the smartphone. For this you can set `CONFIDENCES_NOT_MORE_THAN=Screen:40.0`.   
Also, you can set `CONFIDENCES_NOT_LESS_THAN` to make sure that certain labels exist on the picture.

# DISCOVERY_MODE
In order to retrieve the above described labels, you can enable `DISCOVERY_MODE`, this will not push any MQTT messages, but just output the recognized labels.   
Optionally, you can add `DISCOVERY_LABELS_FILE_OUTPUT` env var and system will save the labels to a separate file.

# Docker-compose example:
```
version: '3'
services:
  recognizer:
    container_name: recognizer
    image: adutchak/recognizer:0.1.0
    volumes:
    - /media/share/enterance_camera_snapshots/:/mnt/
    - /home/root/.aws/credentials:/root/.aws/credentials
    restart: always
    environment:
    - DISCOVERY_MODE=true
    - AWS_REGION=eu-central-1
    - MQTT_TOPIC=enterance/recognizer
    - MQTT_BROKER=10.0.0.150
    - MQTT_PORT=1883
    - MQTT_CLIENT_ID=username
    - MQTT_USERNAME=username
    - MQTT_PASSWORD=massword
    - TARGET_IMAGE_PATH=/mnt/webrtc_screen.jpg
    - SAMPLE_IMAGE_PATHS=/mnt/samples/person1.jpg /mnt/samples/person2.jpg /mnt/samples/person3.jpg
    - SIMILARITY_THRESHOLD=95
    - CONFIDENCES_NOT_LESS_THAN=Photography:98.0,Person:90.0,Indoors:65.0,Face:90.0,Head:90.0,Floor:90.0
    - CONFIDENCES_NOT_MORE_THAN=Electronics:90.0,Phone:40.0,Computer:90.0,Screen:40.0,Computer Hardware:40.0,Hardware:40.0
    - "RECOGNIZED_MESSAGE={\"message\": \"recognized\"}"
    - "NOT_RECOGNIZED_MESSAGE={\"message\": \"not_recognized\"}"
```

# TODO
Make this service as Home Assistant Addon: https://developers.home-assistant.io/docs/add-ons
