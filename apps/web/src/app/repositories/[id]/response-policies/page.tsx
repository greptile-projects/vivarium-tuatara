import type { Metadata } from "next";
import { ResponsePoliciesWorkspace } from "@/components/response-policies-workspace";
export const metadata:Metadata={title:"Response coverage"};
export default async function ResponsePoliciesPage({params}:{params:Promise<{id:string}>}){const{id}=await params;return <ResponsePoliciesWorkspace repositoryID={id}/>}
