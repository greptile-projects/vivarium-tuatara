import type {Metadata} from "next";
import {APIContractsWorkspace} from "@/components/api-contracts-workspace";
export const metadata:Metadata={title:"API contracts"};
export default async function APIContractsPage({params}:{params:Promise<{id:string}>}){const {id}=await params;return <APIContractsWorkspace repositoryID={id}/>}
