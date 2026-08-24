import type {Metadata} from "next";
import {ChangeStacksWorkspace} from "@/components/change-stacks-workspace";
export const metadata:Metadata={title:"Change stacks"};
export default async function Page({params}:{params:Promise<{id:string}>}){const{id}=await params;return <ChangeStacksWorkspace repositoryID={id}/>}
