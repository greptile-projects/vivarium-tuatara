import { OrganizationDetail } from "@/components/organizations-workspace";
export default async function OrganizationPage({params}:{params:Promise<{id:string}>}){const {id}=await params;return <OrganizationDetail organizationID={id}/>}
