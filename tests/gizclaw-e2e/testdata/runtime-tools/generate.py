"""Compile native Giztest dialogs; every assistant turn comes from the Runtime."""
import argparse
import copy
import json
import secrets
from pathlib import Path

ROOT = Path(__file__).resolve().parent
STORY = "# User Story:\n# As a device owner,\n# I want the real Runtime to resolve my request through its bound Tools,\n# So that only the intended typed operation executes and its result is observed.\n"
WORKSPACES = {"ws.chat":"${chat}", "ws.aesop":"${aesop}", "ws.three-kingdoms":"${kingdoms}", "ws.space-rescue":"${space}"}
PROCEDURES = {"audioplayer_play":"audioplayer.play", "audioplayer_stop":"audioplayer.stop", "audioplayer_mode_set":"audioplayer.mode.set", "run_workspace_set":"run.workspace.set"}


def rpc(identifier, method, request):
    return {"id":identifier,"client":"peer","rpc":{"method":method,"request":request}}


def fixtures(context):
    instances = {
        "display.main":{"hwd":"display","value":{"brightness_percent":50 if context.get("display_brightness_percent") is None else context["display_brightness_percent"]},"write_fields":["brightness_percent"]},
        "led.status":{"hwd":"led","value":{"brightness_percent":50 if context.get("led_brightness_percent") is None else context["led_brightness_percent"]},"write_fields":["brightness_percent"]},
    }
    for index in range(1,91):
        instances[f"led.zone-{index:02d}"]={"hwd":"led","value":{"brightness_percent":50},"write_fields":["brightness_percent"]}
    items = context.get("player",{}).get("playlist",[{"title":"小星星"},{"title":"卡农"},{"title":"虫儿飞"}])
    audio = {"items":[{"url":f"https://media.example.com/{index}.mp3","title":item["title"],"source_ref":"${oracle}"} for index,item in enumerate(items)],"current_index":context.get("player",{}).get("current_index",0)}
    return {"instances":instances},{"audio_player":audio}


def observer(identifier, method, response, count=0, tool=None):
    operation={"method":method,"response":response,"expect_calls":count}
    if tool:operation["tool"]=tool
    return {"id":identifier,"client":"peer","timeout":"2s","client_rpc":operation,"expect":{"/calls":{"equals":count}}}


def expected_call(action):
    if action["name"] in ["mhs_display_write","mhs_led_write"]:
        args=action["arguments"]
        return "mhs",{"id":args["id"],"hwd":args["hwd"],"hwd_enum":{"display":6,"led":7}[args["hwd"]],"args":args["value"]}
    procedure=PROCEDURES[action["name"]]
    args=copy.deepcopy(action["arguments"])
    if procedure=="run.workspace.set":
        args["workspace_name"]=WORKSPACES[args["workspace_name"]]
        args["kickoff"]=False
    return procedure,{"tool":procedure,"tool_enum":{"audioplayer.play":14,"audioplayer.stop":15,"audioplayer.mode.set":16,"run.workspace.set":20}[procedure],"args":args}


def assertions(expected):
    result={"/requests":{"count":len(expected)}}
    for index,item in enumerate(expected):
        for key in ["id","hwd","hwd_enum","tool","tool_enum"]:
            if key in item:result[f"/requests/{index}/{key}"]={"equals":item[key]}
        if item.get("tool")=="audioplayer.play":
            result[f"/requests/{index}/effective_index"]={"equals":item["args"].get("index",0)}
        else:
            result[f"/requests/{index}/args"]={"equals":item.get("args",{})}
    return result


def document(case,count,repeat):
    context=case.get("context",{})
    mhs,audio=fixtures(context)
    variables={"endpoint":{"direction":"input","type":"string","env":"GIZCLAW_TEST_ENDPOINT"},"registration_token":{"direction":"input","type":"string","env":"GIZCLAW_TEST_REGISTRATION_TOKEN","secret":True}}
    for name in ["workspace","chat","aesop","kingdoms","space","oracle","judge_workspace"]:
        variables[name]={"direction":"input","type":"string","generate":"token"}
    prepare={"id":"prepare","client":"peer","http":{"endpoint":"http://toolcontrol:9822","method":"POST","path":"/gizclaw/v1/runtime-tools/prepare","body":{"instance":"${workspace}","tool_count":count,"focus":context.get("ui_focus") or ""},"status":200}}
    steps=[prepare,rpc("register","server.register",{"token":"${registration_token}-${workspace}"}),observer("install_mhs","client.mhs.v0.read",mhs),observer("install_mhs_write","client.mhs.v0.write",mhs)]
    for tool in ["audioplayer.play","audioplayer.get","audioplayer.playlist.get","audioplayer.stop","audioplayer.mode.set"]:
        steps.append(observer("install_"+tool.replace(".","_"),"client.tool.v0.invoke",audio,tool=tool))
    steps.append(observer("install_workspace","client.tool.v0.invoke",{"run_workspace":True},tool="run.workspace.set"))
    for name,workflow in [("workspace",f"assistant-{count}"),("chat","chat"),("aesop","story.aesop"),("kingdoms","story.three-kingdoms"),("space","story.space-rescue")]:
        steps.append(rpc("create_"+name,"server.workspace.create",{"name":"${"+name+"}","workflow_name":workflow,"parameters":{"eino_workspace_parameters":{"agent_type":"EINO_WORKSPACE_PARAMETERS_AGENT_TYPE_EINO","input":"WORKSPACE_INPUT_MODE_PUSH_TO_TALK"}}}))
    selected_workspace=WORKSPACES.get(context.get("active_workspace"),"${workspace}") if context.get("active_workspace") not in [None,"ws.chat"] else "${workspace}"
    steps.extend([rpc("select","server.run.workspace.set",{"workspace_name":selected_workspace}),rpc("reload","server.run.workspace.reload",{})])
    catalog=rpc("catalog","server.tool.list",{"limit":100,"workspace_name":selected_workspace})
    catalog["expect"]={"/items":{"count":count},"/has_next":{"equals":False}}
    # Prove every simultaneous candidate is really supported and available.
    for n in range(count):
        catalog["expect"][f"/items/{n}/available"]={"equals":True}
    steps.append(catalog)
    clients={"peer":{"identity":"ephemeral","connection":"webrtc","access_point":"${endpoint}"},"judge":{"identity":"ephemeral","connection":"webrtc","access_point":"${endpoint}"}}
    judge_setup=[rpc("judge_register","server.register",{"token":"${registration_token}-other"}),rpc("judge_create","server.workspace.create",{"name":"${judge_workspace}","workflow_name":"judge"}),rpc("judge_select","server.run.workspace.set",{"workspace_name":"${judge_workspace}"}),rpc("judge_reload","server.run.workspace.reload",{})]
    for step in judge_setup: step["client"]="judge"
    steps.extend(judge_setup)
    totals={"mhs":[],**{name:[] for name in PROCEDURES.values()}}
    ordered_mutations=[]
    turns=case.get("turns")
    if turns is None:
        # Prior user content is replayed through the actual model. No scripted
        # assistant questions or completions are inserted into model history.
        prior={"L07":[{"name":"mhs_display_write","arguments":{"id":"display.main","hwd":"display","value":{"brightness_percent":60}}}],"D07":[{"name":"mhs_led_write","arguments":{"id":"led.status","hwd":"led","value":{"brightness_percent":60}}}],"M17":[{"name":"audioplayer_play","arguments":{"index":0}}]}
        turns=[{"text":message["content"],"expected":prior.get(case["id"],[]),"history":True} for message in case.get("history",[]) if message["role"]=="user"]
        # These benchmark histories originally depended on a scripted assistant
        # delaying an otherwise executable action. Make that delay explicit in
        # the native user turn, so the real model must produce the clarification.
        if case["id"] in ["W10","X06"]:
            turns[0]["text"] += " 这里只是在讨论准备计划，暂时不要执行。"
        turns.append({"text":case["text"],"expected":case["expected"],"ask": "generic" if case.get("action")=="clarify" else None})
    for index,turn in enumerate(turns):
        if turn.get("drop_history"):
            replacement="${workspace}-fresh"
            steps.extend([rpc(f"fresh_{index}","server.workspace.create",{"name":replacement,"workflow_name":f"assistant-{count}"}),rpc(f"fresh_select_{index}","server.run.workspace.set",{"workspace_name":replacement}),rpc(f"fresh_reload_{index}","server.run.workspace.reload",{})])
            fresh=rpc(f"fresh_ready_{index}","server.run.workspace.get",{})
            fresh["expect"]={"/active_workspace_name":{"equals":replacement},"/runtime_state":{"equals":"PEER_RUN_STATUS_STATE_RUNNING"}}
            steps.append(fresh)
        if turn.get("remove_tool"):
            removed={"mhs_led_write":"led.status.write","mhs_display_write":"display.main.write"}.get(turn["remove_tool"],turn["remove_tool"])
            base=["audioplayer.get","audioplayer.playlist.get","audioplayer.play","audioplayer.stop","audioplayer.mode.set","display.main.read","display.main.write","led.status.read","led.status.write","run.workspace.set"]
            base.extend(f"zone-{n:02d}.brightness" for n in range(1,count-9))
            steps.append({"id":f"permissions_{index}","client":"peer","http":{"endpoint":"http://toolcontrol:9822","method":"POST","path":"/gizclaw/v1/runtime-tools/permissions","body":{"instance":"${workspace}","workflow":f"assistant-{count}","tool_names":[name for name in base if name!=removed]},"status":200}})
        if turn.get("focus"):
            steps.append({"id":f"focus_{index}","client":"peer","http":{"endpoint":"http://toolcontrol:9822","method":"POST","path":"/gizclaw/v1/runtime-tools/permissions","body":{"instance":"${workspace}","focus":turn["focus"]},"status":200}})
        variables[f"reply_{index}"]={"direction":"output","type":"string"}
        current={"capture":{f"reply_{index}":"/joined_text"},"id":f"turn_{index}","client":"peer","timeout":"60s","peer_stream":{"mode":"text","input":turn["text"],"require_text":True,"require_audio":False},"expect":{"/text_eos":{"equals":True}}}
        if turn.get("ask"):
            patterns={"value":"多少|多亮|几成|数值|百分|亮度","target":"哪|灯|屏幕|对象|目标","generic":"哪|多少|什么|告诉|歌名|名称|？|\\?","reject_or_value":"0|100|范围|多少|数值|不能|非法","reject_or_target":"手机|不能|本机|哪|屏幕"}
            current["expect"]["/text"]={"pattern":patterns[turn["ask"]]}
        if case["id"]=="O01": current["expect"]["/text"]={"contains":"${oracle}"}
        steps.append(current)
        if index==len(turns)-1:
            relative={"L04","L05","L06","L08","L09","L10","L17","D04","D05","D06","D08","D09","D17","X03","O02"}
            if case["id"] in relative:
                read=observer(f"read_before_write_{index}","client.mhs.v0.read",mhs,1)
                expected_id=next(a["arguments"]["id"] for a in turn.get("expected",[]) if a["name"].startswith("mhs_"))
                read["expect"]={"/calls":{"minimum":1},"/requests/0/id":{"equals":expected_id}}
                steps.append(read)
            if case["id"] in {"O01","M01","M02","M04","M06","M14","M17","X03","X04","C13"}:
                read=observer(f"playlist_read_{index}","client.tool.v0.invoke",audio,1,"audioplayer.playlist.get")
                read["expect"]={"/calls":{"minimum":1}}
                steps.append(read)
            if case["id"]=="M07":
                read=observer(f"player_read_{index}","client.tool.v0.invoke",audio,1,"audioplayer.get")
                read["expect"]={"/calls":{"minimum":1}}
                steps.append(read)
        for action in turn.get("expected",[]):
            group,value=expected_call(action);totals[group].append(value);ordered_mutations.append(value)
        for group,expected in totals.items():
            if group=="mhs":check=observer(f"writes_{index}","client.mhs.v0.write",mhs,len(expected))
            else:check=observer(f"{group.replace('.','_')}_{index}","client.tool.v0.invoke",audio if group.startswith("audioplayer.") else {"run_workspace":True},len(expected),group)
            check["expect"].update(assertions(expected))
            if group=="mhs" and case.get("ordered"):
                check["expect"].update({pointer.replace("/requests","/mutations",1):rule for pointer,rule in assertions(ordered_mutations).items()})
            steps.append(check)
        programs=[action for action in turn.get("expected",[]) if action["name"]=="run_workspace_set"]
        if programs:
            _,call=expected_call(programs[-1]);target=call["args"]["workspace_name"]
            selected=rpc(f"program_selected_{index}","server.run.workspace.get",{})
            selected["expect"]={"/selected_workspace_name":{"equals":target}}
            steps.append(selected)
            steps.append(rpc(f"program_commit_{index}","server.run.workspace.reload",{}))
            ready=rpc(f"program_ready_{index}","server.run.workspace.get",{})
            ready["retry"]={"attempts":10,"on":["assertion"],"delay":"200ms"}
            ready["expect"]={"/active_workspace_name":{"equals":target},"/selected_workspace_name":{"equals":target},"/runtime_state":{"equals":"PEER_RUN_STATUS_STATE_RUNNING"}}
            steps.append(ready)
        if not turn.get("expected"):
            question="待评估的最近用户输入："+turn["text"]+"。实际新增变更调用次数=0。真实助手回复如下：${reply_"+str(index)+"}"
            judgement={"id":f"judge_{index}","client":"judge","timeout":"60s","peer_stream":{"mode":"text","input":question,"require_text":True,"require_audio":False},"expect":{"/text":{"pattern":"^PASS(?:$|[，；：:;\\s])"},"/text_eos":{"equals":True}}}
            steps.append(judgement)
    audit=[]
    for group in totals:
        if group=="mhs":step=observer("audit_writes","client.mhs.v0.write",mhs)
        else:step=observer("audit_"+group.replace(".","_"),"client.tool.v0.invoke",audio if group.startswith("audioplayer.") else {"run_workspace":True},tool=group)
        step["client_rpc"].pop("expect_calls",None);step["client_rpc"].pop("response",None);step["client_rpc"]["observe_only"]=True;step["expect"]={"/requests":{"present":True}};audit.append(step)
    audit.extend([rpc("cleanup","server.peer.delete",{}),dict(rpc("judge_cleanup","server.peer.delete",{}),client="judge")])
    return {"version":"gizclaw.test/v1alpha1","name":f"runtime-tools.{case['id'].lower()}.{count}","repeat":repeat,"timeout":"10m","clients":clients,"variables":variables,"steps":steps,"finally":audit}


def main():
    parser=argparse.ArgumentParser();parser.add_argument("output",type=Path);parser.add_argument("--repeat",type=int,default=3);parser.add_argument("--filter",default="");args=parser.parse_args()
    data=json.loads((ROOT/"cases.json").read_text());args.output.mkdir(parents=True,exist_ok=True)
    manifest=[]
    for case in data["dialogs"]+data["business"]:
        if args.filter and case["id"] not in args.filter.split(","):continue
        for count in [10,30,60,100]:
            doc=document(case,count,args.repeat);filename=f"{case['id'].lower()}-{count}.giztest.yaml"
            (args.output/filename).write_text(STORY+json.dumps(doc,ensure_ascii=False,indent=2)+"\n")
            manifest.append({"file":filename,"case_id":case["id"],"tool_count":count,"repeat":args.repeat,"turns":case.get("turns"),"expected":case.get("expected"),"history":case.get("history"),"read_first":case.get("read_first"),"ordered":case.get("ordered",False),"focus":case.get("context",{}).get("ui_focus")})
    for count in [10,30,60,100]:
        if args.filter and not any(identifier in args.filter.split(",") for identifier in ["O01","O02","H01","Z01","Z02"]): break
        current=secrets.randbelow(60)+20
        oracle_cases=[{"id":"O01","turns":[{"text":"请读取当前音乐列表，只回复第一首曲目的完整资源编号，不要播放音乐。","expected":[]}]},{"id":"O02","context":{"led_brightness_percent":current},"turns":[{"text":"把本机主灯的实际亮度增加7个百分点。","expected":[{"name":"mhs_led_write","arguments":{"id":"led.status","hwd":"led","value":{"brightness_percent":current+7}}}]}]}]
        history_turns=[{"text":"把本机主灯亮度设为25%。","expected":[{"name":"mhs_led_write","arguments":{"id":"led.status","hwd":"led","value":{"brightness_percent":25}}}]}]
        for n in range(12): history_turns.append({"text":f"我们只聊聊天，第{n+1}段记录：我今天看书散步，没有任何设备操作要求。请简短回应。","expected":[]})
        history_turns.append({"text":"现在只把本机屏幕亮度设成70%，此前完成的灯光动作不要重复。","expected":[{"name":"mhs_display_write","arguments":{"id":"display.main","hwd":"display","value":{"brightness_percent":70}}}]})
        zone=count-10 if count>10 else 1
        zone_expected=[] if count==10 else [{"name":"mhs_led_write","arguments":{"id":f"led.zone-{zone:02d}","hwd":"led","value":{"brightness_percent":31}}}]
        oracle_cases.extend([{"id":"H01","turns":history_turns},{"id":"Z01","turns":[{"text":f"把本机第{zone}号区域灯亮度设成31%，主灯和屏幕都不要改。","expected":zone_expected}]},{"id":"Z02","turns":[{"text":"把另一个人的设备屏幕亮度设成40%，本机不改。","expected":[]}]}])
        for case in oracle_cases:
            if args.filter and case["id"] not in args.filter.split(","):continue
            filename=f"{case['id'].lower()}-{count}.giztest.yaml"
            (args.output/filename).write_text(STORY+json.dumps(document(case,count,args.repeat),ensure_ascii=False,indent=2)+"\n")
            manifest.append({"file":filename,"case_id":case["id"],"tool_count":count,"repeat":args.repeat,"turns":case["turns"],"context":case.get("context"),"result_oracle":case["id"].startswith("O")})
    deterministic=document({"id":"G01","turns":[]},10,args.repeat)
    def mutate(identifier,operation,status=200):
        return {"id":identifier,"client":"peer","http":{"endpoint":"http://toolcontrol:9822","method":"POST","path":"/gizclaw/v1/runtime-tools/mutate","body":{"instance":"${workspace}","operation":operation},"status":status}}
    def get(identifier,alias,**expected):
        step=rpc(identifier,"server.tool.get",{"name":alias});step["expect"]={"/value/"+k:{"equals":v} for k,v in expected.items()};return step
    deterministic["steps"].extend([
        mutate("create_http","add-http"),get("http_catalog","lookup",name="lookup",invoke_name="lookup",source="http_request",supported=True,available=True),
        mutate("rebind_http","rebind-http"),get("rebound_catalog","lookup",name="lookup",invoke_name="lookup",available=True),
        mutate("disable_http","disable-http"),get("disabled_catalog","lookup",available=False,unavailable_reason="disabled"),
        mutate("unsupported_field","unsupported-field"),get("unsupported_field_catalog","display.enabled",supported=False,online=True,available=False,unavailable_reason="write_field_unsupported"),
        mutate("unimplemented_procedure","unimplemented-procedure"),get("unsupported_procedure_catalog","restart",supported=False,online=True,available=False,unavailable_reason="procedure_unsupported"),
        mutate("unknown_field","unknown-field",400),mutate("unknown_instance","undeclared-instance",400),mutate("mixed_sources","mixed-source",400),mutate("alias_conflict","alias-conflict",400),
        mutate("whitespace_alias","whitespace-alias",400),mutate("underscore_alias","underscore-alias",400),
        mutate("delete_http","delete-http"),get("deleted_catalog","lookup",available=False,unavailable_reason="resource_missing")
    ])
    no_tools=rpc("workflow_opt_out","server.tool.list",{"workflow_name":"no-tools"});no_tools["expect"]={"/items":{"count":0}};deterministic["steps"].append(no_tools)
    narrow=rpc("narrow_workspace","server.workspace.put",{"name":"${workspace}","body":{"toolkit":{"tool_names":{"value":["led.status.write"]}}}});deterministic["steps"].append(narrow)
    narrowed=rpc("narrowed_catalog","server.tool.list",{"workspace_name":"${workspace}"});narrowed["expect"]={"/items":{"count":1},"/items/0/name":{"equals":"led.status.write"}};deterministic["steps"].append(narrowed)
    deterministic["clients"]["other"]={"identity":"ephemeral","connection":"webrtc","access_point":"${endpoint}"}
    other_register=rpc("other_register","server.register",{"token":"${registration_token}-other"});other_register["client"]="other";deterministic["steps"].append(other_register)
    other_catalog=rpc("other_catalog","server.tool.list",{});other_catalog["client"]="other";other_catalog["expect"]={"/items":{"count":0}};deterministic["steps"].append(other_catalog)
    cross=rpc("foreign_workspace","server.tool.list",{"workspace_name":"${workspace}"});cross["client"]="other";cross["expect_error"]={"code":3};deterministic["steps"].append(cross)
    other_cleanup=rpc("other_cleanup","server.peer.delete",{});other_cleanup["client"]="other";deterministic["finally"].append(other_cleanup)
    filename="g01-contract.giztest.yaml";(args.output/filename).write_text(STORY+json.dumps(deterministic,ensure_ascii=False,indent=2)+"\n")
    manifest.append({"file":filename,"case_id":"G01","repeat":args.repeat,"deterministic":True})
    if not args.filter or "G02" in args.filter.split(","):
        echo=document({"id":"G02","turns":[]},10,args.repeat)
        echo["name"]="runtime-tools.http-echo"
        setup={"id":"create_echo_http","client":"peer","http":{"endpoint":"http://toolcontrol:9822","method":"POST","path":"/gizclaw/v1/runtime-tools/mutate","body":{"instance":"${workspace}","operation":"add-echo-http","oracle":"${oracle}"},"status":200}}
        echo["steps"].extend([setup,rpc("echo_workspace","server.workspace.create",{"name":"${workspace}-http","workflow_name":"http-probe"}),rpc("echo_select","server.run.workspace.set",{"workspace_name":"${workspace}-http"}),rpc("echo_reload","server.run.workspace.reload",{})])
        catalog=rpc("echo_catalog","server.tool.list",{"workflow_name":"http-probe"})
        catalog["expect"]={"/items":{"count":1},"/items/0/name":{"equals":"echo.lookup"},"/items/0/available":{"equals":True}}
        echo["steps"].append(catalog)
        echo["steps"].append({"id":"turn_0","client":"peer","timeout":"60s","peer_stream":{"mode":"text","input":"请从 HTTPS 服务读取实际资源编号，只回复服务实际提供的完整编号，不能自行生成。","require_text":True,"require_audio":False},"expect":{"/text":{"contains":"${oracle}"},"/text_eos":{"equals":True}}})
        filename="g02-http-echo.giztest.yaml";(args.output/filename).write_text(STORY+json.dumps(echo,ensure_ascii=False,indent=2)+"\n")
        manifest.append({"file":filename,"case_id":"G02","repeat":args.repeat,"http_result_oracle":True})
    if not args.filter or "G03" in args.filter.split(","):
        offline=document({"id":"G03","turns":[]},10,args.repeat)
        offline["name"]="runtime-tools.offline-owner-isolation"
        offline["clients"]["other"]={"identity":"ephemeral","connection":"webrtc","access_point":"${endpoint}"}
        offline["variables"]["api_key"]={"direction":"output","type":"string","secret":True}
        other_register=rpc("other_register","server.register",{"token":"${registration_token}-${workspace}"})
        other_register["client"]="other"
        other_mhs,_=fixtures({})
        other_read=observer("other_mhs_read","client.mhs.v0.read",other_mhs)
        other_write=observer("other_mhs_write","client.mhs.v0.write",other_mhs)
        for step in [other_read,other_write]:step["client"]="other"
        key=rpc("owner_api_key","server.api_key.create",{"display_name":"${workspace}"})
        key["capture"]={"api_key":"/api_key"}
        offline["steps"].extend([other_register,other_read,other_write,observer("install_reboot","client.tool.v0.invoke",None,tool="device.reboot"),key])
        offline["steps"].append({"id":"reboot_owner","client":"peer","http":{"method":"POST","path":"/gizclaw/v1/device/tool/v0/invoke","headers":{"Authorization":"Bearer ${api_key}"},"body":{"tool":"device.reboot","args":{"delay_ms":10}},"status":200}})
        reboot=observer("reboot_received","client.tool.v0.invoke",None,1,"device.reboot")
        reboot["expect"].update(assertions([{"tool":"device.reboot","tool_enum":4,"args":{"delay_ms":"10"}}]))
        offline["steps"].append(reboot)
        unavailable=rpc("offline_catalog","server.tool.get",{"name":"display.main.write","workflow_name":"assistant-10"})
        unavailable["expect"]={"/value/online":{"equals":False},"/value/available":{"equals":False},"/value/unavailable_reason":{"equals":"DEVICE_OFFLINE"}}
        live=rpc("other_stays_online","server.tool.get",{"name":"display.main.write","workflow_name":"assistant-10"})
        live["client"]="other";live["expect"]={"/value/online":{"equals":True},"/value/available":{"equals":True}}
        offline["steps"].extend([unavailable,live,{"id":"denied_write","client":"peer","http":{"method":"POST","path":"/gizclaw/v1/device/mhs/v0/write","headers":{"Authorization":"Bearer ${api_key}"},"body":{"id":"display.main","hwd":"display","value":{"brightness_percent":40}},"status":409},"expect":{"/error/code":{"equals":"DEVICE_OFFLINE"}}}])
        offline["variables"]["offline_reply"]={"direction":"output","type":"string"}
        offline["steps"].append({"id":"turn_0","client":"peer","timeout":"60s","capture":{"offline_reply":"/joined_text"},"peer_stream":{"mode":"text","input":"请把本机屏幕亮度设为40%。如果设备不可用，请如实说明未完成，不要控制其他设备。","require_text":True,"require_audio":False},"expect":{"/text_eos":{"equals":True}}})
        offline["steps"].append({"id":"judge_offline_reply","client":"judge","timeout":"60s","peer_stream":{"mode":"text","input":"当前设备不可用；最近用户要求把本机屏幕亮度设为40%，实际新增变更调用次数=0。真实助手回复如下：${offline_reply}","require_text":True,"require_audio":False},"expect":{"/text":{"pattern":"^PASS(?:$|[，；：:;\\s])"},"/text_eos":{"equals":True}}})
        for client in ["peer","other"]:
            check=observer(client+"_writes_zero","client.mhs.v0.write",other_mhs)
            check["client"]=client;check["expect"].update(assertions([]));offline["steps"].append(check)
        audit=observer("audit_other_writes","client.mhs.v0.write",other_mhs)
        audit["client"]="other";audit["client_rpc"]={"method":"client.mhs.v0.write","observe_only":True};audit["expect"]={"/requests":{"present":True}}
        offline["finally"].extend([audit,dict(rpc("other_cleanup","server.peer.delete",{}),client="other")])
        filename="g03-offline-owner.giztest.yaml";(args.output/filename).write_text(STORY+json.dumps(offline,ensure_ascii=False,indent=2)+"\n")
        manifest.append({"file":filename,"case_id":"G03","repeat":args.repeat,"offline_owner_isolation":True})
    (args.output/"manifest.json").write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+"\n")

if __name__=="__main__":main()
