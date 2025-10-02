# ThingsBoard IoT Platform Architecture

This document contains Mermaid diagrams illustrating the ThingsBoard all-in-one platform setup.

## System Architecture Overview

```mermaid
graph TB
    TB[tb-node] --> DB[(postgres)]

    classDef default fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
```

## Original Complex Architecture

```mermaid
graph TB
    subgraph "External Clients"
        WEB[Web Dashboard<br/>:8081]
        MQTT_CLIENT[MQTT Devices<br/>:1884]
        MQTT_SSL[MQTT SSL Devices<br/>:8884]
        COAP[CoAP Devices<br/>:5689-5694/udp]
        EDGE[Edge Devices<br/>:7071]
    end

    subgraph "Docker Network: tb-allinone-network"
        subgraph "ThingsBoard All-in-One Container"
            TB_COMPLEX[ThingsBoard Node<br/>tb-node<br/>Port: 9090]
            
            subgraph "Internal Services"
                API[REST API]
                WS[WebSocket]
                MQTT_BROKER[MQTT Broker<br/>Port: 1883]
                COAP_SERVER[CoAP Server<br/>Port: 5683-5688]
                EDGE_RPC[Edge RPC<br/>Port: 7070]
            end
            
            subgraph "Processing"
                RULE_ENGINE[Rule Engine]
                QUEUE[In-Memory Queue]
                CACHE[Caffeine Cache]
            end
        end
        
        subgraph "PostgreSQL Container"
            DB[(PostgreSQL 16<br/>tb-postgres<br/>Port: 5432<br/>Internal Only)]
        end
    end

    subgraph "Docker Volumes"
        DATA_VOL[tb-allinone-data]
        LOGS_VOL[tb-allinone-logs]  
        DB_VOL[tb-allinone-postgres]
    end

    %% External connections
    WEB --> TB
    MQTT_CLIENT --> MQTT_BROKER
    MQTT_SSL --> MQTT_BROKER
    COAP --> COAP_SERVER
    EDGE --> EDGE_RPC

    %% Internal connections
    TB --> API
    TB --> WS
    TB --> MQTT_BROKER
    TB --> COAP_SERVER
    TB --> EDGE_RPC
    TB --> RULE_ENGINE
    TB --> QUEUE
    TB --> CACHE
    
    %% Database connection
    TB --> DB
    
    %% Volume mounts
    TB -.-> DATA_VOL
    TB -.-> LOGS_VOL
    DB -.-> DB_VOL

    %% Styling
    classDef container fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef database fill:#f3e5f5,stroke:#4a148c,stroke-width:2px
    classDef volume fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef external fill:#e8f5e8,stroke:#2e7d32,stroke-width:2px
    classDef service fill:#fff8e1,stroke:#f57f17,stroke-width:2px

    class TB,MQTT_BROKER,COAP_SERVER,EDGE_RPC container
    class DB database
    class DATA_VOL,LOGS_VOL,DB_VOL volume
    class WEB,MQTT_CLIENT,MQTT_SSL,COAP,EDGE external
    class API,WS,RULE_ENGINE,QUEUE,CACHE service
```

## Port Mapping Details

```mermaid
graph LR
    subgraph "Host System"
        H_WEB[Host :8081]
        H_MQTT[Host :1884]
        H_MQTT_SSL[Host :8884]
        H_EDGE[Host :7071]
        H_COAP[Host :5689-5694/udp]
    end

    subgraph "ThingsBoard Container"
        C_WEB[Container :9090]
        C_MQTT[Container :1883]
        C_MQTT_SSL[Container :8883]
        C_EDGE[Container :7070]
        C_COAP[Container :5683-5688/udp]
    end

    H_WEB --> C_WEB
    H_MQTT --> C_MQTT
    H_MQTT_SSL --> C_MQTT_SSL
    H_EDGE --> C_EDGE
    H_COAP --> C_COAP

    classDef host fill:#ffebee,stroke:#c62828,stroke-width:2px
    classDef container fill:#e3f2fd,stroke:#1565c0,stroke-width:2px
    
    class H_WEB,H_MQTT,H_MQTT_SSL,H_EDGE,H_COAP host
    class C_WEB,C_MQTT,C_MQTT_SSL,C_EDGE,C_COAP container
```

## Data Flow Diagram

```mermaid
sequenceDiagram
    participant Device as IoT Device
    participant MQTT as MQTT Broker
    participant TB as ThingsBoard Core
    participant RE as Rule Engine  
    participant DB as PostgreSQL
    participant UI as Web Dashboard

    Device->>MQTT: Publish telemetry/attributes
    MQTT->>TB: Forward device data
    TB->>RE: Process through rule chains
    RE->>DB: Store processed data
    TB->>DB: Store device metadata
    
    UI->>TB: Request dashboard data
    TB->>DB: Query telemetry/attributes
    DB->>TB: Return data
    TB->>UI: Send dashboard data
    
    Note over Device,UI: Real-time updates via WebSocket
```

## Service Dependencies

```mermaid
graph TD
    START([Docker Compose Up])
    
    START --> PG_START[Start PostgreSQL Container]
    PG_START --> PG_HEALTH{PostgreSQL<br/>Health Check}
    PG_HEALTH -->|Pass| TB_START[Start ThingsBoard Container]
    PG_HEALTH -->|Fail| PG_RETRY[Wait & Retry]
    PG_RETRY --> PG_HEALTH
    
    TB_START --> TB_INIT[Initialize ThingsBoard]
    TB_INIT --> TB_HEALTH{ThingsBoard<br/>Health Check}
    TB_HEALTH -->|Pass| READY[System Ready]
    TB_HEALTH -->|Fail| TB_RETRY[Wait & Retry]
    TB_RETRY --> TB_HEALTH
    
    READY --> ACCESS[Access via :8081]

    classDef start fill:#c8e6c9,stroke:#388e3c,stroke-width:2px
    classDef process fill:#fff3e0,stroke:#f57c00,stroke-width:2px
    classDef decision fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px
    classDef end fill:#ffcdd2,stroke:#d32f2f,stroke-width:2px

    class START,READY,ACCESS start
    class PG_START,TB_START,TB_INIT,PG_RETRY,TB_RETRY process
    class PG_HEALTH,TB_HEALTH decision
```

## Configuration Overview

| Component | Configuration | Value |
|-----------|---------------|-------|
| **ThingsBoard** | Service Type | Monolith (All-in-One) |
| | Queue Type | In-Memory |
| | Cache Type | Caffeine |
| | Memory | 768MB-1536MB |
| **PostgreSQL** | Version | 16 |
| | Database | thingsboard |
| | User/Password | postgres/postgres |
| | Access | Internal only |
| **Network** | Driver | Bridge |
| | Name | tb-allinone-network |
| **Volumes** | Data | tb-allinone-data |
| | Logs | tb-allinone-logs |
| | Database | tb-allinone-postgres |

## Access URLs

- **Web Dashboard**: http://localhost:8081
- **MQTT Endpoint**: localhost:1884
- **MQTT SSL Endpoint**: localhost:8884  
- **CoAP Endpoint**: localhost:5689-5694/udp
- **Edge RPC**: localhost:7071

## Health Checks

- **PostgreSQL**: `pg_isready -U postgres` (every 30s)
- **ThingsBoard**: `curl -f http://localhost:9090/login` (every 45s, 7min startup grace period)