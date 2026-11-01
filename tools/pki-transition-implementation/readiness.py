"""Future custody/consent/startup candidates. No real authority, store write or task registration."""
import xml.etree.ElementTree as ET
from fence import Stop
def custody_candidate(plan):
    required={"CA_credential_record","backup_credential_record","copy_A","copy_B","recovered_credential_test",
              "archive_server_key_restore_test","physical_separation_attested","independent_review_sha256"}
    if set(plan)!=required:raise Stop("custody-plan-fields")
    if not all(isinstance(plan[k],str) and plan[k] for k in ("CA_credential_record","backup_credential_record","copy_A","copy_B","independent_review_sha256")):
        raise Stop("custody-plan-reference")
    if plan["CA_credential_record"]==plan["backup_credential_record"] or plan["copy_A"]==plan["copy_B"]:
        raise Stop("custody-plan-separation")
    missing=[k for k in ("recovered_credential_test","archive_server_key_restore_test","physical_separation_attested") if plan[k] is not True]
    return {"candidate_complete":not missing,"missing":missing,"real_creation_authorized":False,
            "Owner_must_select_actual_vault_and_media":True}
def startup_task_proposal(owner_sid,monitor_path):
    if not owner_sid.startswith("S-1-") or not monitor_path.startswith("SYNTHETIC-"):
        raise Stop("startup-proposal-fixture-only")
    task=ET.Element("Task",version="1.4")
    settings=ET.SubElement(task,"Settings");ET.SubElement(settings,"Enabled").text="false"
    ET.SubElement(settings,"ExecutionTimeLimit").text="PT2M"
    principals=ET.SubElement(task,"Principals");p=ET.SubElement(principals,"Principal",id="Owner")
    ET.SubElement(p,"UserId").text=owner_sid;ET.SubElement(p,"LogonType").text="InteractiveToken"
    ET.SubElement(p,"RunLevel").text="LeastPrivilege"
    triggers=ET.SubElement(task,"Triggers");t=ET.SubElement(triggers,"LogonTrigger")
    ET.SubElement(t,"Enabled").text="false";ET.SubElement(t,"UserId").text=owner_sid
    actions=ET.SubElement(task,"Actions",Context="Owner");a=ET.SubElement(actions,"Exec")
    ET.SubElement(a,"Command").text=monitor_path
    ET.SubElement(a,"Arguments").text="--public-status-only --no-signing --no-service-start"
    return ET.tostring(task,encoding="unicode")
