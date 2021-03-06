*** Settings ***
Documentation  This resource contains any keywords dealing with operations being performed on a Vsphere instance, mostly govc wrappers

*** Keywords ***
Power On VM OOB
    [Arguments]  ${vm}
    ${rc}  ${output}=  Run Keyword If  '%{HOST_TYPE}' == 'VC'  Run And Return Rc And Output  govc vm.power -on %{VCH-NAME}/"${vm}"
    Run Keyword If  '%{HOST_TYPE}' == 'VC'  Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Run And Return Rc And Output  govc vm.power -on "${vm}"
    Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Should Be Equal As Integers  ${rc}  0
    Log To Console  Waiting for VM to power on ...
    Wait Until VM Powers On  ${vm}

Power Off VM OOB
    [Arguments]  ${vm}
    ${rc}  ${output}=  Run Keyword If  '%{HOST_TYPE}' == 'VC'  Run And Return Rc And Output  govc vm.power -off %{VCH-NAME}/"${vm}"
    Run Keyword If  '%{HOST_TYPE}' == 'VC'  Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Run And Return Rc And Output  govc vm.power -off "${vm}"
    Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Should Be Equal As Integers  ${rc}  0
    Log To Console  Waiting for VM to power off ...
    Wait Until VM Powers Off  "${vm}"

Destroy VM OOB
    [Arguments]  ${vm}
    ${rc}  ${output}=  Run Keyword If  '%{HOST_TYPE}' == 'VC'  Run And Return Rc And Output  govc vm.destroy %{VCH-NAME}/"*-${vm}"
    Run Keyword If  '%{HOST_TYPE}' == 'VC'  Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Run And Return Rc And Output  govc vm.destroy "*-${vm}"
    Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Should Be Equal As Integers  ${rc}  0

Put Host Into Maintenance Mode
    ${rc}  ${output}=  Run And Return Rc And Output  govc host.maintenance.enter -host.ip=%{TEST_URL}
    Should Contain  ${output}  entering maintenance mode... OK

Remove Host From Maintenance Mode
    ${rc}  ${output}=  Run And Return Rc And Output  govc host.maintenance.exit -host.ip=%{TEST_URL}
    Should Contain  ${output}  exiting maintenance mode... OK

Wait Until VM Powers On
    [Arguments]  ${vm}
    :FOR  ${idx}  IN RANGE  0  30
    \   ${ret}=  Run Keyword If  '%{HOST_TYPE}' == 'VC'  Run  govc vm.info %{VCH-NAME}/${vm}
    \   Run Keyword If  '%{HOST_TYPE}' == 'VC'  Set Test Variable  ${out}  ${ret}
    \   ${ret}=  Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Run  govc vm.info ${vm}
    \   Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Set Test Variable  ${out}  ${ret}
    \   ${status}=  Run Keyword And Return Status  Should Contain  ${out}  poweredOn
    \   Return From Keyword If  ${status}
    \   Sleep  1
    Fail  VM did not power on within 30 seconds

Wait Until VM Powers Off
    [Arguments]  ${vm}
    :FOR  ${idx}  IN RANGE  0  30
    \   ${ret}=  Run Keyword If  '%{HOST_TYPE}' == 'VC'  Run  govc vm.info %{VCH-NAME}/${vm}
    \   Run Keyword If  '%{HOST_TYPE}' == 'VC'  Set Test Variable  ${out}  ${ret}
    \   ${ret}=  Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Run  govc vm.info ${vm}
    \   Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Set Test Variable  ${out}  ${ret}
    \   ${status}=  Run Keyword And Return Status  Should Contain  ${out}  poweredOff
    \   Return From Keyword If  ${status}
    \   Sleep  1
    Fail  VM did not power off within 30 seconds

Wait Until VM Is Destroyed
    [Arguments]  ${vm}
    :FOR  ${idx}  IN RANGE  0  30
    \   ${ret}=  Run Keyword If  '%{HOST_TYPE}' == 'VC'  Run  govc ls vm/%{VCH-NAME}/${vm}
    \   Run Keyword If  '%{HOST_TYPE}' == 'VC'  Set Test Variable  ${out}  ${ret}
    \   ${ret}=  Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Run  govc ls vm/${vm}
    \   Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Set Test Variable  ${out}  ${ret}
    \   ${status}=  Run Keyword And Return Status  Should Be Empty  ${out}
    \   Return From Keyword If  ${status}
    \   Sleep  1
    Fail  VM was not destroyed within 30 seconds

Get VM IP
    [Arguments]  ${vm}
    ${rc}  ${out}=  Run And Return Rc And Output  govc vm.ip ${vm}
    Should Be Equal As Integers  ${rc}  0
    [Return]  ${out}

Get VM Name
    [Arguments]  ${vm}
    ${ret}=  Run Keyword If  '%{HOST_TYPE}' == 'VC'  Set Variable  ${vm}/${vm}
    ${ret}=  Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Set Variable  ${vm}
    [Return]  ${ret}

Get VM Host Name
    [Arguments]  ${vm}
    ${vm}=  Get VM Name  ${vm}
    ${ret}=  Run  govc vm.info ${vm}/${vm}
    Set Test Variable  ${out}  ${ret}
    ${out}=  Split To Lines  ${out}
    ${host}=  Fetch From Right  @{out}[-1]  ${SPACE}
    [Return]  ${host}

Get VM Info
    [Arguments]  ${vm}
    ${rc}  ${out}=  Run And Return Rc And Output  govc vm.info -r ${vm}
    Should Be Equal As Integers  ${rc}  0
    [Return]  ${out}

Create Test Server Snapshot
    [Arguments]  ${vm}  ${snapshot}
    Set Environment Variable  GOVC_URL  %{BUILD_SERVER}
    ${rc}  ${out}=  Run And Return Rc And Output  govc snapshot.create -vm ${vm} ${snapshot}
    Should Be Equal As Integers  ${rc}  0
    Should Be Empty  ${out}
    Set Environment Variable  GOVC_URL  %{TEST_USERNAME}:%{TEST_PASSWORD}@%{TEST_URL}

Revert Test Server Snapshot
    [Arguments]  ${vm}  ${snapshot}
    Set Environment Variable  GOVC_URL  %{BUILD_SERVER}
    ${rc}  ${out}=  Run And Return Rc And Output  govc snapshot.revert -vm ${vm} ${snapshot}
    Should Be Equal As Integers  ${rc}  0
    Should Be Empty  ${out}
    Set Environment Variable  GOVC_URL  %{TEST_USERNAME}:%{TEST_PASSWORD}@%{TEST_URL}

Delete Test Server Snapshot
    [Arguments]  ${vm}  ${snapshot}
    Set Environment Variable  GOVC_URL  %{BUILD_SERVER}
    ${rc}  ${out}=  Run And Return Rc And Output  govc snapshot.remove -vm ${vm} ${snapshot}
    Should Be Equal As Integers  ${rc}  0
    Should Be Empty  ${out}
    Set Environment Variable  GOVC_URL  %{TEST_USERNAME}:%{TEST_PASSWORD}@%{TEST_URL}

Setup Snapshot
    ${hostname}=  Get Test Server Hostname
    Set Environment Variable  TEST_HOSTNAME  ${hostname}
    Set Environment Variable  SNAPSHOT  vic-ci-test-%{DRONE_BUILD_NUMBER}
    Create Test Server Snapshot  %{TEST_HOSTNAME}  %{SNAPSHOT}

Get Datacenter Name
    ${out}=  Run  govc datacenter.info
    ${out}=  Split To Lines  ${out}
    ${name}=  Fetch From Right  @{out}[0]  ${SPACE}
    [Return]  ${name}

Get Test Server Hostname
    [Tags]  secret
    ${hostname}=  Run  sshpass -p $TEST_PASSWORD ssh $TEST_USERNAME@$TEST_URL hostname
    [Return]  ${hostname}

Check Delete Success
    [Arguments]  ${name}
    ${out}=  Run  govc ls vm
    Log  ${out}
    Should Not Contain  ${out}  ${name}
    ${out}=  Run  govc datastore.ls
    Log  ${out}
    Should Not Contain  ${out}  ${name}
    ${out}=  Run  govc ls host/*/Resources/*
    Log  ${out}
    Should Not Contain  ${out}  ${name}

Gather Logs From ESX Server
    Environment Variable Should Be Set  TEST_URL
    ${out}=  Run  govc logs.download

Change Log Level On Server
    [Arguments]  ${level}
    ${out}=  Run  govc host.option.set Config.HostAgent.log.level ${level}
    Should Be Empty  ${out}
