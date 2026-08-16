import type {Metadata} from "next";
import {RecoveryCommitmentsWorkspace} from "@/components/recovery-commitments-workspace";

export const metadata:Metadata={title:"Recovery commitments"};
export default async function RecoveryPage({params}:{params:Promise<{id:string}>}){const {id}=await params;return <RecoveryCommitmentsWorkspace repositoryID={id}/>}
