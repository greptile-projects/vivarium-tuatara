import {CharterWorkspace} from "@/components/charter-workspace";
export default async function Page({params}:{params:Promise<{id:string}>}){const {id}=await params;return <CharterWorkspace kind="repository" id={id}/>}
