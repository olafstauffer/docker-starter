# docker-starter

Tool to modify config files based on environment variables on docker startup.

## Why?

Some applications do not allow to specify the connection configuration they need via the command arguments and therefore cannot be configured via the docker cmd arguments. Usually those application need some config files on startup.

A typical pattern is to provide those config files via docker volume to the application. Unfortunatelly this can become very messy setting up complex dev environments. Especially when a connection string is the only information that needs to be modified and we want to use docker links (or the link within a fig.yml). This usually results in a lot of local config files that have to be manually synced with a fig.yml.


## What does it do?

  * meant to be used as docker CMD - runs on container start
  * processes templates within config files
  * processes templates within command line args
  * replaces tempate markup with environment variables
  * starts the actual container application (additional args are passed)
  * forwards signals to the running application


## Usage

    docker-starter -cmd COMMAND -dir DIR [-force] [--] [ADDITIONAL ARGS]
   
    -cmd="": command to execute
    -dir="": directory to read templates (*.tmpl) and write output to
    -force=false: overwrite existing files

## Examples


#### Environment with Elasticsearch and Kibana

Run kibana on port 8000 and connect it to the linked elasticsearch container.

#### fig.yml
    kibana:
        image: ollo/kibana
        ports:
        - "8000:8000"
        links:
        - elasticsearch
        environment:
        - KIBANA_PORT=8000
    elasticsearch:
        image: dockerfile/elasticsearch
        ports:
        - 9200
        - 9300

#### .../kibana/config/kibana.yml.tmpl (inside docker image)
    ...
    elasticsearch: "{{E .ELASTICSEARCH_9200_URL}}"
    port: {{if .KIBANA_PORT}}{{E .KIBANA_PORT}}{{else}}5601{{end}}
    ...

Note:

 * This also works when multiple elasticsearch nodes are started (e.g. fig scale elasticsearch=3), because with function _E_ only the first element is used. 

 * If the environment variable _KIBANA_PORT_ is not given it will use 5601 as default.


#### Creating Kibana image with docker-starter

    Dockerfile
        ...
        ENV KIBANA_VERSION 4.0.0-beta3
        ...
        COPY kibana.yml.tmpl /opt/kibana-$KIBANA_VERSION/config/
        COPY docker-starter /usr/local/bin/ 
        CMD ["docker-starter", "-cmd", "/opt/kibana-{{E .KIBANA_VERSION}}/bin/kibana", "-dir", "/opt/kibana-{{E .KIBANA_VERSION}}/config"]


## Internals

#### Internal Data Structure

Internally the simple key/value list of environment variables is transformed to a map of string slices. Therefore every key can have multiple values associated with it.

This map is the central data structure used by the templating engine.


#### Templates

The go templating engine is used. (For more info on the engine see see: http://golang.org/pkg/text/template/).

Additionally there are two template pipeline functions to make it easy to work with the internal data structure (the map of string slices).

##### E  
Returns the first value element for a key (or "" if the key does not exist)  
Example: {{E .FOO}} gives "BAR" if value is set to ["BAR", "IT", "IS"]

##### J [sep]  
Returns a the value elements joined by the separator (default to ',')  
Example: {{J .FOO "#"}} gives "BAR#IT#IS" if value is set to ["BAR", "IT", "IS"] 

#### Link Variables

*fig* set's environment variables automatically when linking containers.

e.g. with the fig.yml above the following variables are created:

    ELASTICSEARCH_1_ENV_ES_PKG_NAME=elasticsearch-1.4.1
    ELASTICSEARCH_1_ENV_JAVA_HOME=/usr/lib/jvm/java-7-oracle
    ELASTICSEARCH_1_NAME=/test_kibana_1/elasticsearch_1
    ELASTICSEARCH_1_PORT=tcp://172.17.0.32:9200
    ELASTICSEARCH_1_PORT_9200_TCP=tcp://172.17.0.32:9200
    ELASTICSEARCH_1_PORT_9200_TCP_ADDR=172.17.0.32
    ELASTICSEARCH_1_PORT_9200_TCP_PORT=9200
    ELASTICSEARCH_1_PORT_9200_TCP_PROTO=tcp
    ELASTICSEARCH_1_PORT_9300_TCP=tcp://172.17.0.32:9300
    ELASTICSEARCH_1_PORT_9300_TCP_ADDR=172.17.0.32
    ELASTICSEARCH_1_PORT_9300_TCP_PORT=9300
    ELASTICSEARCH_1_PORT_9300_TCP_PROTO=tcp
    KIBANA_PORT=8000

Variables with a key that matches the regexp _"^([^_]+)_(\d*).*PORT_(\d+)_TCP$"_ are then processed. The regexp returns a CONTAINER, a CONTAINERINDEX and a PORT that this container provides. The value of the key provides a SCHEMA, a HOST and a PORT. 

Two internal keys are generated with this info:  
 * "$CONTAINER_URL"
 * "$CONTAINER_$PORT_URL"

Both of them contain a list of values in the form "http://HOST:PORT" (the SCHEMA is ignored). 

So when there are more than one container of the same type linked to a application,  

e.g. two additional instances of elasticsearch  

    ELASTICSEARCH_1_PORT_9200_TCP=tcp://172.17.0.32:9200
    ELASTICSEARCH_2_PORT_9200_TCP=tcp://172.17.0.33:9200
    ELASTICSEARCH_3_PORT_9200_TCP=tcp://172.17.0.34:9200

The variable _ELASTICSEARCH_9200_URL_ contains the list: "http://172.17.0.32:9200", "http://172.17.0.33:9200", "http://172.17.0.34:9200".

(Using a template {{J .ELASTICSEARCH_9200_URL}} this will result in the string "http://172.17.0.32:9200,http://172.17.0.33:9200,http://172.17.0.34:9200").


#### Signals

After running the command the main execution is blocked and waits for the command to exit. Every signal is forwared to the command.

Note: After sending a KILL Signal (-9) the running command must be manually destroyed, because a KILL cannot be forwarded.

