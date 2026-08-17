import type {Metadata} from "next";
import {DurableStateWorkspace} from "@/components/durable-state-workspace";
export const metadata:Metadata={title:"Durable state"};
export default async function Page({params}:{params:Promise<{id:string}>}){const {id}=await params;return <DurableStateWorkspace repositoryID={id}/>}
