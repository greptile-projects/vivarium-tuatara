import type {Metadata} from "next";
import {GovernanceWorkspace} from "@/components/governance-workspace";
export const metadata:Metadata={title:"Governance"};
export default function GovernancePage(){return <GovernanceWorkspace/>}
