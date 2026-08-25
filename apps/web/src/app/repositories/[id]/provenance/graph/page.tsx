import type {Metadata} from "next";
import {ProvenanceGraphsWorkspace} from "@/components/provenance-graphs-workspace";
export const metadata:Metadata={title:"Revision-exact provenance"};
export default async function Page({params}:{params:Promise<{id:string}>}){const{id}=await params;return <ProvenanceGraphsWorkspace repositoryID={id}/>}
