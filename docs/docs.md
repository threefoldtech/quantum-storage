# QSFS

```mermaid
C4Component
title qsfs with 0-db's on Threefold grid 

Deployment_Node(qsfssystem, "system with qsfs",""){
    Component(zdbfs, "0-db fs")
    Component(localzorodb,"Local 0-db")
    Component(zstor, "Zstor")
}

Deployment_Node(node1,"Node 1", ""){
    Deployment_Node(vm1,"VM", ""){
        Component(zerodb1,"0-db 1")
    }
}
Deployment_Node(node2,"Node 1", ""){
    Deployment_Node(vm2,"VM", ""){
        Component(zerodb2,"0-db 2")
    }
}
Deployment_Node(nodex,"Node ...", ""){ 
    Deployment_Node(vmx,"VM", ""){
        Component(zerodbx,"0-db ...")
    }
}

Deployment_Node(noden,"Node n", ""){
    Deployment_Node(vmn,"VM", ""){
        Component(zerodbn,"0-db N")
    }
}

Rel(zstor, zerodb1, "")
Rel(zstor, zerodb2, "")
Rel(zstor, zerodbx, "")
Rel(zstor, zerodbn, "")
```
