import type {Metadata} from "next";
import {ProvenanceAssessmentsWorkspace} from "@/components/provenance-assessments-workspace";
export const metadata:Metadata={title:"Provenance readiness"};
export default async function Page({params}:{params:Promise<{id:string}>}){const{id}=await params;return <ProvenanceAssessmentsWorkspace repositoryID={id}/>}
